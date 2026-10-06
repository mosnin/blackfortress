//go:build unix

package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// Proc is a supervised child process whose output goes to a log file.
type Proc struct {
	Name string
	cmd  *exec.Cmd
	done chan struct{}
	err  error
}

func startProc(name string, cmd *exec.Cmd, logPath string) (*Proc, error) {
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}

	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, fmt.Errorf("cannot start %s: %w", name, err)
	}

	p := &Proc{Name: name, cmd: cmd, done: make(chan struct{})}
	go func() {
		p.err = cmd.Wait()
		logFile.Close()
		close(p.done)
	}()

	return p, nil
}

// Pid returns the process id.
func (p *Proc) Pid() int {
	if p == nil || p.cmd.Process == nil {
		return 0
	}

	return p.cmd.Process.Pid
}

// Done is closed when the process exits.
func (p *Proc) Done() <-chan struct{} { return p.done }

func (p *Proc) Exited() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

// Stop sends sig and escalates to SIGKILL on the process group after timeout.
func (p *Proc) Stop(sig syscall.Signal, timeout time.Duration) {
	if p == nil || p.Exited() {
		return
	}

	_ = p.cmd.Process.Signal(sig)

	select {
	case <-p.done:
	case <-time.After(timeout):
		_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
		<-p.done
	}
}
