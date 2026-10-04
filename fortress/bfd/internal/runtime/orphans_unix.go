//go:build unix

package runtime

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func writePidFile(dir, name string, pid int) {
	if pid <= 0 || os.MkdirAll(dir, 0o700) != nil {
		return
	}

	_ = os.WriteFile(filepath.Join(dir, name+".pid"), []byte(strconv.Itoa(pid)), 0o600)
}

func removePidFile(dir, name string) {
	_ = os.Remove(filepath.Join(dir, name+".pid"))
}

// killOrphans stops processes a previous bfd started and never reaped:
// everything recorded in run/*.pid, plus the postmaster recorded by
// postgres itself. A pid is only signalled while it still runs the
// expected program, so a recycled pid is never touched.
func killOrphans(runDir, pgData string, logf func(string, ...any)) {
	candidates := map[int]string{}

	files, _ := filepath.Glob(filepath.Join(runDir, "*.pid"))
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
				candidates[pid] = strings.TrimSuffix(filepath.Base(f), ".pid")
			}
		}

		_ = os.Remove(f)
	}

	if pid := postmasterPid(pgData); pid > 0 {
		candidates[pid] = "postgres"
	}

	for pid, name := range candidates {
		if !runs(pid, name) {
			continue
		}

		logf("stopping orphaned %s (pid %d) from a previous run", name, pid)

		sig := syscall.SIGTERM
		if name == "postgres" {
			sig = syscall.SIGINT
		}

		_ = syscall.Kill(pid, sig)

		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) && alive(pid) {
			time.Sleep(200 * time.Millisecond)
		}

		if alive(pid) {
			_ = syscall.Kill(-pid, syscall.SIGKILL)
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}
}

func postmasterPid(pgData string) int {
	f, err := os.Open(filepath.Join(pgData, "postmaster.pid"))
	if err != nil {
		return 0
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return 0
	}

	pid, _ := strconv.Atoi(strings.TrimSpace(sc.Text()))

	return pid
}

func alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// runs reports whether pid is alive and its executable name contains name.
func runs(pid int, name string) bool {
	if !alive(pid) {
		return false
	}

	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil {
		return false
	}

	return strings.Contains(strings.ToLower(filepath.Base(strings.TrimSpace(string(out)))), strings.ToLower(name))
}
