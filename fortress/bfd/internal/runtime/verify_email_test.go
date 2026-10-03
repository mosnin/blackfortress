package runtime

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewestTokenDecodesQuotedPrintable(t *testing.T) {
	dir := t.TempDir()

	// A soft line break inside the token must not truncate it.
	msg := "Subject: Confirm your email address\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/alternative; boundary=BOUND\r\n\r\n" +
		"--BOUND\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"Content-Transfer-Encoding: quoted-printable\r\n\r\n" +
		"Open http://localhost:7810/auth/verify-email?token=3DabcDEF.ghi=\r\nJKL_mno\r\n" +
		"--BOUND--\r\n"

	if err := os.WriteFile(filepath.Join(dir, "1.eml"), []byte(msg), 0o600); err != nil {
		t.Fatal(err)
	}

	if got := newestToken(dir, time.Now().Add(-time.Minute)); got != "abcDEF.ghiJKL_mno" {
		t.Fatalf("token = %q", got)
	}

	if got := newestToken(dir, time.Now().Add(time.Minute)); got != "" {
		t.Fatalf("old mail should be ignored, got %q", got)
	}
}

func TestParseRPCCalls(t *testing.T) {
	calls := parseRPCCalls([]byte(`[{"method":"tools/call","params":{"name":"listRisks","arguments":{"a":1}}},{"method":"ping"}]`))
	if len(calls) != 2 || calls[0].Params.Name != "listRisks" {
		t.Fatalf("calls = %+v", calls)
	}

	if mcpControls("listRisks") != nil || mcpControls("createRisk") == nil {
		t.Fatal("read-only tools must not be tagged; writes must be")
	}
}
