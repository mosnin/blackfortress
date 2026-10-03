package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"blackfortress.dev/fortress/bfd/internal/paths"
	"blackfortress.dev/fortress/bfd/internal/secrets"
)

// MCPStdio bridges a stdio MCP client to bfd's streamable-HTTP endpoint, for
// agents that only launch MCP servers as subprocesses.
func MCPStdio(stdin io.Reader, stdout io.Writer) int {
	home, err := paths.Home()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	sec, err := secrets.Load(paths.NewLayout(home).Secrets)
	if err != nil {
		fmt.Fprintln(os.Stderr, "black fortress is not set up yet: start the app or run `bfd run` first")
		return 1
	}

	var (
		mu        sync.Mutex
		sessionID string
	)

	out := bufio.NewWriter(stdout)
	emit := func(msg []byte) {
		mu.Lock()
		defer mu.Unlock()

		_, _ = out.Write(bytes.TrimSpace(msg))
		_ = out.WriteByte('\n')
		_ = out.Flush()
	}

	client := &http.Client{Timeout: 10 * time.Minute}

	sc := bufio.NewScanner(stdin)
	sc.Buffer(make([]byte, 1<<20), 32<<20)

	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}

		req, err := http.NewRequest(http.MethodPost, controlURL()+"/mcp", bytes.NewReader(append([]byte(nil), line...)))
		if err != nil {
			continue
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Authorization", "Bearer "+sec.MCPToken)
		req.Header.Set("User-Agent", "bf-mcp-stdio")

		mu.Lock()
		if sessionID != "" {
			req.Header.Set("Mcp-Session-Id", sessionID)
		}
		mu.Unlock()

		resp, err := client.Do(req)
		if err != nil {
			emitError(emit, line, "Black Fortress is not running: "+err.Error())
			continue
		}

		if id := resp.Header.Get("Mcp-Session-Id"); id != "" {
			mu.Lock()
			sessionID = id
			mu.Unlock()
		}

		relay(resp, line, emit)
	}

	return 0
}

func relay(resp *http.Response, request []byte, emit func([]byte)) {
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusAccepted || resp.StatusCode == http.StatusNoContent {
		return
	}

	ct := resp.Header.Get("Content-Type")

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		emitError(emit, request, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body))))

		return
	}

	if strings.HasPrefix(ct, "text/event-stream") {
		rd := bufio.NewReader(resp.Body)

		var data bytes.Buffer
		for {
			line, err := rd.ReadString('\n')
			trimmed := strings.TrimRight(line, "\r\n")

			switch {
			case strings.HasPrefix(trimmed, "data:"):
				data.WriteString(strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")))
			case trimmed == "" && data.Len() > 0:
				emit(data.Bytes())
				data.Reset()
			}

			if err != nil {
				if data.Len() > 0 {
					emit(data.Bytes())
				}

				return
			}
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err == nil && len(bytes.TrimSpace(body)) > 0 {
		emit(body)
	}
}

// emitError answers a request with a JSON-RPC error so the client doesn't
// hang. Notifications (no id) get no answer.
func emitError(emit func([]byte), request []byte, msg string) {
	var req struct {
		ID json.RawMessage `json:"id"`
	}
	if json.Unmarshal(request, &req) != nil || len(req.ID) == 0 || string(req.ID) == "null" {
		return
	}

	b, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      req.ID,
		"error":   map[string]any{"code": -32000, "message": msg},
	})
	emit(b)
}
