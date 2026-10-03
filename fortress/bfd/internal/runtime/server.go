package runtime

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"blackfortress.dev/fortress/bfd/internal/guard"
)

// Server is bfd's loopback control API. See fortress/ARCHITECTURE.md.
type Server struct {
	d   *Daemon
	srv *http.Server

	nonceMu sync.Mutex
	nonces  map[string]time.Time
}

func newServer(d *Daemon) *Server {
	return &Server{d: d, nonces: map[string]time.Time{}}
}

func (s *Server) Start(port int) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/status", s.status)
	mux.HandleFunc("GET /v1/events", s.events)
	mux.HandleFunc("GET /v1/posture", s.posture)
	mux.HandleFunc("GET /v1/ledger", s.ledger)
	mux.HandleFunc("GET /v1/ledger/verify", s.verifyLedger)
	mux.HandleFunc("POST /v1/hooks/{event}", s.requireToken(s.hook))
	mux.HandleFunc("POST /v1/evidence/sync", s.requireToken(s.syncEvidence))
	mux.HandleFunc("GET /v1/login-link", s.loginLink)
	mux.HandleFunc("GET /login", s.login)
	mux.Handle("/mcp", s.requireToken(s.mcpProxy()))
	mux.Handle("/mcp/", s.requireToken(s.mcpProxy()))

	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return err
	}

	s.srv = &http.Server{Handler: rejectForeignOrigins(mux), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = s.srv.Serve(ln) }()

	return nil
}

// rejectForeignOrigins blocks browser requests from non-local origins. The
// API only listens on loopback, but a web page could still try to reach it
// from the user's browser.
func rejectForeignOrigins(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1") {
				http.Error(w, "forbidden origin", http.StatusForbidden)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		want := s.d.secrets.MCPToken

		if want == "" || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="blackfortress"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)

			return
		}

		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) status(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.d.Status())
}

func (s *Server) posture(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.d.Posture())
}

func (s *Server) ledger(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	entries, err := s.d.ledger.Recent(limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if entries == nil {
		entries = []guard.Entry{}
	}

	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

func (s *Server) verifyLedger(w http.ResponseWriter, _ *http.Request) {
	n, err := s.d.ledger.Verify()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"valid": false, "entries": n, "error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"valid": true, "entries": n})
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, unsubscribe := s.d.hub.Subscribe()
	defer unsubscribe()

	status, _ := json.Marshal(Event{Type: "status", Time: time.Now().UTC(), Data: s.d.Status()})
	_, _ = w.Write([]byte("event: status\ndata: " + string(status) + "\n\n"))
	flusher.Flush()

	keepalive := time.NewTicker(20 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case frame, ok := <-ch:
			if !ok {
				return
			}

			_, _ = w.Write(frame)
			flusher.Flush()
		case <-keepalive.C:
			_, _ = w.Write([]byte(": keepalive\n\n"))
			flusher.Flush()
		}
	}
}

// hook records a guardrail evaluation that `bf hook` already made, so the
// ledger and live events stay consistent even when bfd was briefly down.
func (s *Server) hook(w http.ResponseWriter, r *http.Request) {
	var e guard.Entry
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&e); err != nil {
		http.Error(w, "invalid entry", http.StatusBadRequest)
		return
	}

	s.d.publishEntry(e)
	w.WriteHeader(http.StatusNoContent)
}

// syncEvidence uploads agent evidence now, including a snapshot of today.
func (s *Server) syncEvidence(w http.ResponseWriter, r *http.Request) {
	if s.d.Status().State != "running" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "runtime not running"})
		return
	}

	n, err := s.d.SyncEvidence(r.Context(), true)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"uploaded": n, "error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"uploaded": n})
}

func (s *Server) loginLink(w http.ResponseWriter, _ *http.Request) {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	nonce := hex.EncodeToString(b)

	s.nonceMu.Lock()
	now := time.Now()
	for k, exp := range s.nonces {
		if now.After(exp) {
			delete(s.nonces, k)
		}
	}
	s.nonces[nonce] = now.Add(60 * time.Second)
	s.nonceMu.Unlock()

	writeJSON(w, http.StatusOK, map[string]string{
		"url": "http://localhost:" + strconv.Itoa(s.d.cfg.ControlPort) + "/login?nonce=" + nonce,
	})
}

func (s *Server) consumeNonce(nonce string) bool {
	s.nonceMu.Lock()
	defer s.nonceMu.Unlock()

	exp, ok := s.nonces[nonce]
	delete(s.nonces, nonce)

	return ok && time.Now().Before(exp)
}

// login signs the local user in to probod and hands the session cookie to
// the browser. Cookies ignore ports, so a cookie set by localhost:7811 is
// sent to the console on localhost:7810.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.consumeNonce(r.URL.Query().Get("nonce")) {
		http.Error(w, "login link expired; reopen the console from Black Fortress", http.StatusForbidden)
		return
	}

	sess := NewSession(s.d.probodURL())
	if err := sess.SignIn(r.Context(), s.d.secrets.UserEmail, s.d.secrets.UserPassword); err != nil {
		http.Error(w, "sign-in failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	if org := s.d.secrets.OrganizationID; org != "" {
		if err := sess.AssumeOrganization(r.Context(), org); err != nil {
			s.d.logger.Printf("login: %v", err)
		}
	}

	for _, c := range sess.Cookies() {
		http.SetCookie(w, &http.Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
	}

	target := s.d.probodURL() + "/"
	if org := s.d.secrets.OrganizationID; org != "" {
		target = s.d.probodURL() + "/organizations/" + url.PathEscape(org)
	}

	http.Redirect(w, r, target, http.StatusFound)
}

// mcpProxy forwards agent MCP traffic to probod with the provisioned agent
// token and records every tool call in the ledger.
func (s *Server) mcpProxy() http.HandlerFunc {
	upstream, _ := url.Parse(s.d.probodURL())

	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(upstream)
			pr.Out.URL.Path = "/api/mcp/v1/"
			pr.Out.URL.RawPath = ""
			pr.Out.Host = upstream.Host
			pr.Out.Header.Set("Authorization", "Bearer "+s.d.secrets.APIKey)
			pr.Out.Header.Del("Cookie")
		},
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			http.Error(w, "Black Fortress runtime is not ready: "+err.Error(), http.StatusBadGateway)
		},
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.Body != nil {
			body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
			if err != nil {
				http.Error(w, "cannot read request", http.StatusBadRequest)
				return
			}

			r.Body = io.NopCloser(bytes.NewReader(body))
			r.ContentLength = int64(len(body))
			s.d.recordMCPCalls(r.Header.Get("User-Agent"), body)
		}

		proxy.ServeHTTP(w, r)
	}
}

type rpcCall struct {
	Method string `json:"method"`
	Params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"params"`
}

func parseRPCCalls(body []byte) []rpcCall {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return nil
	}

	if body[0] == '[' {
		var batch []rpcCall
		if json.Unmarshal(body, &batch) == nil {
			return batch
		}

		return nil
	}

	var one rpcCall
	if json.Unmarshal(body, &one) == nil {
		return []rpcCall{one}
	}

	return nil
}
