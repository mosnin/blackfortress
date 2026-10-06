//go:build unix

package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"blackfortress.dev/fortress/bfd/internal/paths"
)

// Up starts `bfd run` in the background (if it is not already running) and
// waits until the runtime is ready. Meant for setup scripts of headless
// machines: cloud agent sandboxes, containers, CI.
func Up(args []string, stdout io.Writer) int {
	fs := flag.NewFlagSet("up", flag.ContinueOnError)
	timeout := fs.Duration("timeout", 3*time.Minute, "how long to wait for the runtime to be ready")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	home, err := paths.Home()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	layout := paths.NewLayout(home)

	if st, ok := currentStatus(); ok && st.State == "running" {
		printReady(stdout, st)
		return 0
	}

	var exited chan error

	if _, ok := currentStatus(); !ok {
		if exited, err = spawnDaemon(layout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}

		fmt.Fprintf(stdout, "Starting Black Fortress (data in %s)…\n", home)
	}

	deadline := time.Now().Add(*timeout)
	for time.Now().Before(deadline) {
		select {
		case err := <-exited:
			fmt.Fprintf(os.Stderr, "bfd exited during startup (%v); see %s\n", err, filepath.Join(layout.Logs, "bfd.log"))
			return 1
		case <-time.After(time.Second):
		}

		st, ok := currentStatus()
		if !ok {
			continue
		}

		switch st.State {
		case "running":
			printReady(stdout, st)
			return 0
		case "error":
			fmt.Fprintf(os.Stderr, "Black Fortress failed to start: %s\nsee %s\n", st.Error, filepath.Join(layout.Logs, "bfd.log"))
			return 1
		}
	}

	fmt.Fprintf(os.Stderr, "Black Fortress was not ready after %s; see %s\n", *timeout, filepath.Join(layout.Logs, "bfd.log"))

	return 1
}

// Down stops a runtime started by `bf up` (or any `bfd run` on this data
// directory) and waits for it to exit.
func Down(stdout io.Writer) int {
	home, err := paths.Home()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	layout := paths.NewLayout(home)

	data, err := os.ReadFile(layout.PidFile)
	if errors.Is(err, os.ErrNotExist) {
		if _, ok := currentStatus(); ok {
			fmt.Fprintln(os.Stderr, "a runtime is answering but has no pid file here (another BF_HOME?); stop it where it was started")
			return 1
		}

		fmt.Fprintln(stdout, "Black Fortress is not running.")

		return 0
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		fmt.Fprintf(os.Stderr, "unreadable pid file %s\n", layout.PidFile)
		return 1
	}

	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		_ = os.Remove(layout.PidFile)
		fmt.Fprintln(stdout, "Black Fortress is not running (removed a stale pid file).")

		return 0
	}

	for deadline := time.Now().Add(90 * time.Second); time.Now().Before(deadline); {
		if syscall.Kill(pid, 0) != nil {
			fmt.Fprintln(stdout, "✓ Black Fortress stopped")
			return 0
		}

		time.Sleep(300 * time.Millisecond)
	}

	fmt.Fprintf(os.Stderr, "bfd (pid %d) did not stop within 90s\n", pid)

	return 1
}

// spawnDaemon starts `bfd run` in its own session so it outlives the
// calling shell, and reports when it exits.
func spawnDaemon(layout paths.Layout) (chan error, error) {
	bfd, err := paths.FindBinary("bfd")
	if err != nil {
		return nil, err
	}

	if err := layout.Ensure(); err != nil {
		return nil, err
	}

	out, err := os.OpenFile(filepath.Join(layout.Logs, "bfd-console.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	defer out.Close()

	cmd := exec.Command(bfd, "run")
	cmd.Stdout = out
	cmd.Stderr = out
	cmd.Dir = layout.Home
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("cannot start %s: %w", bfd, err)
	}

	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	return exited, nil
}

type statusView struct {
	State      string `json:"state"`
	Error      string `json:"error"`
	ConsoleURL string `json:"console_url"`
	MCPURL     string `json:"mcp_url"`
	Version    string `json:"version"`
}

func currentStatus() (statusView, bool) {
	var st statusView

	resp, err := (&http.Client{Timeout: 3 * time.Second}).Get(controlURL() + "/v1/status")
	if err != nil {
		return st, false
	}
	defer resp.Body.Close()

	return st, json.NewDecoder(resp.Body).Decode(&st) == nil
}

func printReady(w io.Writer, st statusView) {
	fmt.Fprintf(w, "✓ Black Fortress %s is running\n  console: %s\n  MCP:     %s (agents: `bf install-claude` or `bf agent-config`)\n", st.Version, st.ConsoleURL, st.MCPURL)
}
