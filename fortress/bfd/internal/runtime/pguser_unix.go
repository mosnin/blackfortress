//go:build unix

package runtime

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
)

// defaultPgUser is the system account PostgreSQL runs as when bfd is root.
const defaultPgUser = "blackfortress"

// pgCredential returns the identity initdb and postgres run as, or nil to
// run them as bfd's own user. PostgreSQL refuses to run as root, and cloud
// sandboxes and containers usually are root, so in that case it uses an
// unprivileged account (BF_PG_USER, default "blackfortress"), creating it
// on Linux when it does not exist.
func pgCredential() (*syscall.Credential, error) {
	if os.Geteuid() != 0 {
		return nil, nil
	}

	name := os.Getenv("BF_PG_USER")
	if name == "" {
		name = defaultPgUser
	}

	u, err := user.Lookup(name)
	if err != nil {
		if err := createSystemUser(name); err != nil {
			return nil, err
		}

		if u, err = user.Lookup(name); err != nil {
			return nil, fmt.Errorf("created user %q but cannot look it up: %w", name, err)
		}
	}

	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("user %q has a non-numeric uid %q", name, u.Uid)
	}

	gid, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("user %q has a non-numeric gid %q", name, u.Gid)
	}

	if uid == 0 {
		return nil, fmt.Errorf("BF_PG_USER %q is root; PostgreSQL needs an unprivileged user", name)
	}

	return &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}, nil
}

func createSystemUser(name string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("PostgreSQL cannot run as root and user %q does not exist: run bfd as a normal user or create it", name)
	}

	attempts := [][]string{
		{"useradd", "--system", "--no-create-home", "--shell", "/usr/sbin/nologin", name},
		{"adduser", "-S", "-D", "-H", name}, // busybox (Alpine)
	}

	var errs []error
	for _, args := range attempts {
		path, err := exec.LookPath(args[0])
		if err != nil {
			continue
		}

		out, err := exec.Command(path, args[1:]...).CombinedOutput()
		if err == nil {
			return nil
		}

		errs = append(errs, fmt.Errorf("%s: %w: %s", args[0], err, out))
	}

	if len(errs) == 0 {
		return fmt.Errorf("PostgreSQL cannot run as root, user %q does not exist and neither useradd nor adduser is available", name)
	}

	return fmt.Errorf("cannot create user %q for PostgreSQL: %w", name, errors.Join(errs...))
}

// handOver gives the PostgreSQL user the pg directory (data, socket dir,
// password file) and makes sure it can reach it through every parent.
func handOver(pgDir string, cred *syscall.Credential) error {
	if cred == nil {
		return nil
	}

	err := filepath.WalkDir(pgDir, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		return os.Lchown(path, int(cred.Uid), int(cred.Gid))
	})
	if err != nil {
		return fmt.Errorf("cannot hand %s to the PostgreSQL user: %w", pgDir, err)
	}

	// The data directory's parent ($BF_HOME) holds root-only secrets, so it
	// only gets search permission: enough to reach pg/, not to list or read.
	home := filepath.Dir(pgDir)
	if info, err := os.Stat(home); err == nil && info.Mode().Perm()&0o001 == 0 {
		if err := os.Chmod(home, info.Mode().Perm()|0o011); err != nil {
			return err
		}
	}

	return reachable(filepath.Dir(home))
}

// reachable reports an error naming the first directory on the way to path
// that the PostgreSQL user cannot search.
func reachable(path string) error {
	for dir := path; ; dir = filepath.Dir(dir) {
		info, err := os.Stat(dir)
		if err != nil {
			return err
		}

		if info.IsDir() && info.Mode().Perm()&0o001 == 0 {
			return fmt.Errorf("the PostgreSQL user cannot reach %s: %s is not searchable by others (install under /opt and keep BF_HOME under /var/lib)", path, dir)
		}

		if dir == filepath.Dir(dir) {
			return nil
		}
	}
}
