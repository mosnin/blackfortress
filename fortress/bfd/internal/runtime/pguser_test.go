//go:build unix

package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReachableNamesTheBlockingDirectory(t *testing.T) {
	root := t.TempDir()
	private := filepath.Join(root, "private")
	target := filepath.Join(private, "postgres", "bin")

	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := reachable(target); err != nil && !strings.Contains(err.Error(), "not searchable") {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := os.Chmod(private, 0o700); err != nil {
		t.Fatal(err)
	}

	err := reachable(target)
	if err == nil || !strings.Contains(err.Error(), private+" is not searchable") {
		t.Fatalf("want an error naming %s, got %v", private, err)
	}
}

func TestPgCredentialOnlyWhenRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("covered by the root smoke test")
	}

	cred, err := pgCredential()
	if err != nil || cred != nil {
		t.Fatalf("non-root bfd must run postgres as itself: %v %v", cred, err)
	}
}
