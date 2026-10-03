package runtime

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3afero"
	"github.com/spf13/afero"
)

const (
	storageBucket    = "probod"
	storageAccessKey = "blackfortress"
	storageRegion    = "us-east-1"
)

// Storage is an in-process S3-compatible server backed by a local folder.
// probod talks to it exactly as it would to S3 or SeaweedFS.
type Storage struct {
	Dir  string
	Port int

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

	s.srv = &http.Server{Handler: faker.Server(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := s.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Println("object storage stopped:", err)
		}
	}()

	return nil
}

func (s *Storage) Stop(ctx context.Context) {
	if s.srv != nil {
		_ = s.srv.Shutdown(ctx)
	}
}
