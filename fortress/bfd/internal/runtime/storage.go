package runtime

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3afero"
	"github.com/spf13/afero"
)

const (
	storageBucket = "probod"
	storageRegion = "us-east-1"
)

// Storage is an in-process S3-compatible server backed by a local folder.
// probod talks to it exactly as it would to S3 or SeaweedFS.
type Storage struct {
	Dir  string
	Port int
	// KeyID must appear in every request's credential scope (SigV4 header
	// or presigned X-Amz-Credential). gofakes3 itself verifies nothing.
	KeyID string
	// ConsoleOrigin is the only browser origin allowed (presigned downloads).
	ConsoleOrigin string

	srv *http.Server
}

func (s *Storage) Endpoint() string { return "http://127.0.0.1:" + strconv.Itoa(s.Port) }

func (s *Storage) Start() error {
	fs := afero.NewBasePathFs(afero.NewOsFs(), s.Dir)

	backend, err := s3afero.MultiBucket(fs)
	if err != nil {
		return fmt.Errorf("cannot open object store: %w", err)
	}

	exists, err := backend.BucketExists(storageBucket)
	if err != nil {
		return err
	}

	if !exists {
		if err := backend.CreateBucket(storageBucket); err != nil {
			return fmt.Errorf("cannot create bucket: %w", err)
		}
	}

	faker := gofakes3.New(backend, gofakes3.WithoutVersioning())

	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(s.Port))
	if err != nil {
		return fmt.Errorf("cannot listen for object storage: %w", err)
	}

	s.srv = &http.Server{Handler: s.guard(faker.Server()), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := s.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Println("object storage stopped:", err)
		}
	}()

	return nil
}

// guard makes the fake S3 server safe to run on a developer machine: it
// requires the per-install key id, refuses browser origins other than the
// console, and drops gofakes3's permissive CORS headers.
func (s *Storage) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if r.Method == http.MethodOptions || (origin != "" && origin != s.ConsoleOrigin) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		if s.KeyID == "" || !hasKeyID(r, s.KeyID) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		next.ServeHTTP(corsStripper{ResponseWriter: w, origin: origin}, r)
	})
}

func hasKeyID(r *http.Request, keyID string) bool {
	if cred := r.URL.Query().Get("X-Amz-Credential"); strings.HasPrefix(cred, keyID+"/") {
		return true
	}

	return strings.Contains(r.Header.Get("Authorization"), "Credential="+keyID+"/")
}

// corsStripper replaces gofakes3's wildcard CORS headers: only the console
// origin may read responses.
type corsStripper struct {
	http.ResponseWriter
	origin string
}

func (c corsStripper) fix() {
	h := c.ResponseWriter.Header()
	for k := range h {
		if strings.HasPrefix(http.CanonicalHeaderKey(k), "Access-Control-") {
			h.Del(k)
		}
	}

	if c.origin != "" {
		h.Set("Access-Control-Allow-Origin", c.origin)
		h.Set("Vary", "Origin")
	}
}

func (c corsStripper) WriteHeader(code int) {
	c.fix()
	c.ResponseWriter.WriteHeader(code)
}

func (c corsStripper) Write(b []byte) (int, error) {
	c.fix()
	return c.ResponseWriter.Write(b)
}

func (c corsStripper) Flush() {
	if f, ok := c.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *Storage) Stop(ctx context.Context) {
	if s.srv != nil {
		_ = s.srv.Shutdown(ctx)
	}
}
