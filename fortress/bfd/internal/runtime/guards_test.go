package runtime

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, DELETE")
		w.WriteHeader(http.StatusOK)
	})
}

func TestRequireLocalHost(t *testing.T) {
	h := requireLocalHost(7811, okHandler())

	for host, want := range map[string]int{
		"localhost:7811":        http.StatusOK,
		"127.0.0.1:7811":        http.StatusOK,
		"[::1]:7811":            http.StatusOK,
		"attacker.example":      http.StatusForbidden,
		"attacker.example:7811": http.StatusForbidden,
		"localhost:7810":        http.StatusForbidden,
	} {
		req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != want {
			t.Errorf("Host %q: %d, want %d", host, rec.Code, want)
		}
	}
}

func TestStorageGuard(t *testing.T) {
	s := &Storage{KeyID: "KEY123", ConsoleOrigin: "http://localhost:7810"}
	h := s.guard(okHandler())

	signed := "AWS4-HMAC-SHA256 Credential=KEY123/20261004/us-east-1/s3/aws4_request, SignedHeaders=host, Signature=x"

	cases := []struct {
		name   string
		method string
		target string
		auth   string
		origin string
		want   int
	}{
		{"unsigned", http.MethodPut, "/probod/x", "", "", http.StatusForbidden},
		{"wrong key", http.MethodGet, "/probod/x", "AWS4-HMAC-SHA256 Credential=OTHER/20261004/us-east-1/s3/aws4_request", "", http.StatusForbidden},
		{"foreign origin", http.MethodGet, "/probod/x", signed, "https://evil.example", http.StatusForbidden},
		{"preflight", http.MethodOptions, "/probod/x", signed, "http://localhost:7810", http.StatusForbidden},
		{"probod", http.MethodPut, "/probod/x", signed, "", http.StatusOK},
		{"presigned from console", http.MethodGet, "/probod/x?X-Amz-Credential=KEY123%2F20261004%2Fus-east-1%2Fs3%2Faws4_request", "", "http://localhost:7810", http.StatusOK},
	}

	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.target, nil)
		if tc.auth != "" {
			req.Header.Set("Authorization", tc.auth)
		}

		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != tc.want {
			t.Errorf("%s: %d, want %d", tc.name, rec.Code, tc.want)
			continue
		}

		if rec.Code == http.StatusOK {
			acao := rec.Header().Get("Access-Control-Allow-Origin")
			if acao == "*" || (tc.origin == "" && acao != "") || rec.Header().Get("Access-Control-Allow-Methods") != "" {
				t.Errorf("%s: permissive CORS headers leaked: %v", tc.name, rec.Header())
			}
		}
	}
}
