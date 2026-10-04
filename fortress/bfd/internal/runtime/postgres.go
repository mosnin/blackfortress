package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	pgUser     = "postgres"
	pgDatabase = "probod"
)

type Postgres struct {
	BinDir   string
	DataDir  string
	RunDir   string
	Port     int
	Password string
	LogPath  string
}

func (p *Postgres) Addr() string { return "127.0.0.1:" + strconv.Itoa(p.Port) }

// Init runs initdb once.
func (p *Postgres) Init(ctx context.Context) error {
	if _, err := os.Stat(filepath.Join(p.DataDir, "PG_VERSION")); err == nil {
		return nil
	}

	if err := os.MkdirAll(p.DataDir, 0o700); err != nil {
		return err
	}

	pwFile := filepath.Join(filepath.Dir(p.DataDir), ".pwfile")
	if err := os.WriteFile(pwFile, []byte(p.Password+"\n"), 0o600); err != nil {
		return err
	}
	defer os.Remove(pwFile)

	cmd := exec.CommandContext(
		ctx,
		filepath.Join(p.BinDir, "initdb"),
		"-D", p.DataDir,
		"-U", pgUser,
		"--pwfile", pwFile,
		"--auth", "scram-sha-256",
		"--encoding", "UTF8",
		"--locale", "C",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("initdb failed: %w\n%s", err, out)
	}

	return nil
}

// Start launches the postgres server process and waits until it accepts
// connections.
func (p *Postgres) Start(ctx context.Context) (*Proc, error) {
	// A stale pid file left by a crash blocks startup. Postgres refuses to
	// start if the recorded process is still alive, so removing the file only
	// matters when the old server is really gone.
	pidFile := filepath.Join(p.DataDir, "postmaster.pid")
	if _, err := os.Stat(pidFile); err == nil && !p.ready(ctx) {
		_ = os.Remove(pidFile)
	}

	cmd := exec.Command(
		filepath.Join(p.BinDir, "postgres"),
		"-D", p.DataDir,
		"-p", strconv.Itoa(p.Port),
		"-k", p.RunDir,
		"-c", "listen_addresses=127.0.0.1",
		"-c", "max_connections=200",
		"-c", "shared_buffers=128MB",
	)

	proc, err := startProc("postgres", cmd, p.LogPath)
	if err != nil {
		return nil, err
	}

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if p.ready(ctx) {
			// A server left over from a previous run answers too; make sure
			// the one we started is the one serving.
			select {
			case <-proc.Done():
				return nil, errors.New("postgres exited right after start (port or data directory in use?); see " + p.LogPath)
			case <-time.After(500 * time.Millisecond):
			}

			return proc, p.ensureDatabase(ctx)
		}

		select {
		case <-proc.Done():
			return nil, errors.New("postgres exited during startup; see " + p.LogPath)
		case <-ctx.Done():
			proc.Stop(syscall.SIGINT, 10*time.Second)
			return nil, ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}

	proc.Stop(syscall.SIGINT, 10*time.Second)

	return nil, errors.New("postgres did not become ready in time; see " + p.LogPath)
}

func (p *Postgres) connString(db string) string {
	return fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", pgUser, p.Password, p.Addr(), db)
}

func (p *Postgres) ready(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, p.connString("postgres"))
	if err != nil {
		return false
	}
	defer conn.Close(ctx)

	return conn.Ping(ctx) == nil
}

func (p *Postgres) ensureDatabase(ctx context.Context) error {
	conn, err := pgx.Connect(ctx, p.connString("postgres"))
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	var exists bool
	if err := conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", pgDatabase).Scan(&exists); err != nil {
		return err
	}

	if exists {
		return nil
	}

	_, err = conn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{pgDatabase}.Sanitize())

	return err
}
