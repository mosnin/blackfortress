// Package paths resolves the Black Fortress data directory and the location
// of bundled binaries.
package paths

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

const (
	ProbodPort  = 7810
	ControlPort = 7811
	StoragePort = 7812
	MailPort    = 7813
	PgPort      = 7814
)

// Home returns the data directory, honoring BF_HOME.
func Home() (string, error) {
	if h := os.Getenv("BF_HOME"); h != "" {
		return filepath.Abs(h)
	}

	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot resolve home directory: %w", err)
	}

	if runtime.GOOS == "darwin" {
		return filepath.Join(userHome, "Library", "Application Support", "BlackFortress"), nil
	}

	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "blackfortress"), nil
	}

	return filepath.Join(userHome, ".local", "share", "blackfortress"), nil
}

// Layout is the set of well-known paths inside the data directory.
type Layout struct {
	Home      string
	Secrets   string
	Config    string
	PgData    string
	PgRun     string
	Objects   string
	Mail      string
	Ledger    string
	Policy    string
	Logs      string
	StateFile string
}

func NewLayout(home string) Layout {
	return Layout{
		Home:      home,
		Secrets:   filepath.Join(home, "secrets.json"),
		Config:    filepath.Join(home, "probod.yml"),
		PgData:    filepath.Join(home, "pg", "data"),
		PgRun:     filepath.Join(home, "pg", "run"),
		Objects:   filepath.Join(home, "objects"),
		Mail:      filepath.Join(home, "mail"),
		Ledger:    filepath.Join(home, "ledger"),
		Policy:    filepath.Join(home, "policy.json"),
		Logs:      filepath.Join(home, "logs"),
		StateFile: filepath.Join(home, "state.json"),
	}
}

func (l Layout) Ensure() error {
	for _, dir := range []string{l.Home, l.PgRun, l.Objects, l.Mail, l.Ledger, l.Logs} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("cannot create %s: %w", dir, err)
		}
	}

	return nil
}

// FindBinary looks for name in BF_BIN_DIR, next to the running executable,
// then on PATH.
func FindBinary(name string) (string, error) {
	var candidates []string
	if dir := os.Getenv("BF_BIN_DIR"); dir != "" {
		candidates = append(candidates, filepath.Join(dir, name))
	}

	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), name))
	}

	for _, c := range candidates {
		if isExecutable(c) {
			return c, nil
		}
	}

	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}

	return "", fmt.Errorf("cannot find %q (set BF_BIN_DIR or add it to PATH)", name)
}

// FindPostgresBinDir locates a directory containing initdb and postgres.
func FindPostgresBinDir() (string, error) {
	var candidates []string
	if dir := os.Getenv("BF_PG_DIR"); dir != "" {
		candidates = append(candidates, filepath.Join(dir, "bin"), dir)
	}

	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "..", "postgres", "bin"))
	}

	for _, v := range []string{"17", "16", "15"} {
		candidates = append(
			candidates,
			"/usr/lib/postgresql/"+v+"/bin",
			"/opt/homebrew/opt/postgresql@"+v+"/bin",
			"/usr/local/opt/postgresql@"+v+"/bin",
			"/Applications/Postgres.app/Contents/Versions/"+v+"/bin",
		)
	}

	for _, c := range candidates {
		if isExecutable(filepath.Join(c, "initdb")) && isExecutable(filepath.Join(c, "postgres")) {
			return filepath.Clean(c), nil
		}
	}

	if p, err := exec.LookPath("initdb"); err == nil {
		return filepath.Dir(p), nil
	}

	return "", errors.New("cannot find PostgreSQL binaries (set BF_PG_DIR)")
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}
