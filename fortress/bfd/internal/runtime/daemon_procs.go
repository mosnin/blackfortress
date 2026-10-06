package runtime

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"blackfortress.dev/fortress/bfd/internal/paths"
)

type Config struct {
	ProbodPort  int
	ControlPort int
	StoragePort int
	MailPort    int
	PgPort      int
}

func DefaultConfig() Config {
	return Config{
		ProbodPort:  envPort("BF_PROBOD_PORT", paths.ProbodPort),
		ControlPort: envPort("BF_CONTROL_PORT", paths.ControlPort),
		StoragePort: envPort("BF_STORAGE_PORT", paths.StoragePort),
		MailPort:    envPort("BF_MAIL_PORT", paths.MailPort),
		PgPort:      envPort("BF_PG_PORT", paths.PgPort),
	}
}

func envPort(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return v
	}

	return def
}

func (d *Daemon) startProbod(ctx context.Context) error {
	d.setService("probod", "starting")

	if err := d.probod.WriteConfig(ctx); err != nil {
		return err
	}

	proc, err := d.probod.Start(ctx)
	if err != nil {
		d.setService("probod", "down")
		return err
	}

	d.track("probod", proc)
	d.setService("probod", "up")

	return nil
}

func (d *Daemon) track(name string, p *Proc) {
	d.mu.Lock()
	d.procs[name] = p
	d.mu.Unlock()

	writePidFile(d.runDir(), name, p.Pid())
}

func (d *Daemon) stopProc(name string, sig syscall.Signal) {
	d.mu.Lock()
	p := d.procs[name]
	delete(d.procs, name)
	d.mu.Unlock()

	if p != nil {
		p.Stop(sig, 20*time.Second)
	}

	removePidFile(d.runDir(), name)
}

func (d *Daemon) runDir() string { return filepath.Join(d.layout.Home, "run") }

// supervise keeps postgres and probod running. It restarts whichever is
// missing (postgres first, since probod depends on it) with exponential
// backoff, and never gives up while bfd runs.
func (d *Daemon) supervise(ctx context.Context) {
	backoff := time.Second

	wait := func() bool {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(backoff):
		}

		if backoff < time.Minute {
			backoff *= 2
		}

		return true
	}

	for {
		d.mu.Lock()
		pg, pd := d.procs["postgres"], d.procs["probod"]
		d.mu.Unlock()

		switch {
		case pg == nil:
			d.setService("postgres", "restarting")

			p, err := d.pg.Start(ctx)
			if err != nil {
				d.logger.Printf("postgres restart failed: %v", err)
				if !wait() {
					return
				}

				continue
			}

			d.track("postgres", p)
			d.setService("postgres", "up")

			continue
		case pd == nil:
			if err := d.startProbod(ctx); err != nil {
				d.logger.Printf("probod restart failed: %v", err)
				if !wait() {
					return
				}

				continue
			}

			backoff = time.Second
			d.setState("running", "")

			continue
		}

		select {
		case <-ctx.Done():
			return
		case <-pg.Done():
			d.logger.Printf("postgres exited; restarting")
			d.setState("degraded", "postgres exited")
			d.stopProc("postgres", syscall.SIGINT)
			// probod holds connections to the old server; restart it too.
			d.stopProc("probod", syscall.SIGTERM)
		case <-pd.Done():
			d.logger.Printf("probod exited; restarting")
			d.setState("degraded", "probod exited")
			d.setService("probod", "restarting")
			d.stopProc("probod", syscall.SIGTERM)
		}

		if !wait() {
			return
		}
	}
}

// startChrome launches a headless Chrome for PDF export, only when the user
// opts in with BF_ENABLE_PDF=1: Chrome's DevTools port has no
// authentication, so any local process could drive the browser with the
// user's file access. It listens on a random loopback port.
func (d *Daemon) startChrome() string {
	if os.Getenv("BF_ENABLE_PDF") != "1" {
		return ""
	}

	bin := findChrome()
	if bin == "" {
		return ""
	}

	port, err := freePort()
	if err != nil {
		d.logger.Printf("chrome unavailable: %v", err)
		return ""
	}

	cmd := exec.Command(
		bin,
		"--headless=new",
		"--disable-gpu",
		"--no-first-run",
		"--no-default-browser-check",
		"--remote-debugging-address=127.0.0.1",
		"--remote-debugging-port="+strconv.Itoa(port),
		"--user-data-dir="+filepath.Join(d.layout.Home, "chrome"),
	)

	p, err := startProc("chrome", cmd, filepath.Join(d.layout.Logs, "chrome.log"))
	if err != nil {
		d.logger.Printf("chrome unavailable: %v", err)
		return ""
	}

	d.track("chrome", p)
	d.setService("chrome", "up")

	return "127.0.0.1:" + strconv.Itoa(port)
}

func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()

	return ln.Addr().(*net.TCPAddr).Port, nil
}

func findChrome() string {
	if v := os.Getenv("BF_CHROME"); v != "" {
		return v
	}

	candidates := []string{"google-chrome", "chromium", "chromium-browser"}
	if runtime.GOOS == "darwin" {
		candidates = append([]string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
		}, candidates...)
	}

	for _, c := range candidates {
		if filepath.IsAbs(c) {
			if _, err := os.Stat(c); err == nil {
				return c
			}

			continue
		}

		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}

	return ""
}

func (d *Daemon) shutdown() {
	d.setState("stopping", "")
	d.stopProc("probod", syscall.SIGTERM)
	d.stopProc("chrome", syscall.SIGTERM)
	// SIGINT is postgres "fast" shutdown: it rolls back open transactions
	// and checkpoints, unlike SIGTERM's "smart" wait for clients.
	d.stopProc("postgres", syscall.SIGINT)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if d.storage != nil {
		d.storage.Stop(ctx)
	}

	if d.mail != nil {
		d.mail.Stop()
	}
}

func writeDefaultPolicy(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	// The user file starts empty: built-in rules apply, and any rule added
	// here with the same id overrides the default.
	const starter = `{
  "version": 1,
  "rules": []
}
`

	return os.WriteFile(path, []byte(starter), 0o600)
}
