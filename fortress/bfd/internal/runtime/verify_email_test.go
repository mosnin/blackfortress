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

func TestMatchControls(t *testing.T) {
	controls := []orgControl{
		{ID: "soc-cc81", SectionTitle: "CC8.1", Framework: "SOC 2"},
		{ID: "soc-cc61", SectionTitle: "CC6.1", Framework: "SOC 2"},
		{ID: "iso-a832", SectionTitle: "A.8.32", Framework: "ISO 27001 (2022)"},
		{ID: "iso-74", SectionTitle: "7.4", Framework: "ISO 27001 (2022)"},
		{ID: "pims-74", SectionTitle: "7.4", Framework: "ISO/IEC 27701:2025"},
		{ID: "gdpr-25-1", SectionTitle: "Art. 25(1)", Framework: "GDPR"},
		{ID: "gdpr-250", SectionTitle: "Art. 250", Framework: "GDPR"},
	}

	got := matchControls([]string{"SOC2:CC8.1", "ISO27001:A.8.32", "ISO27701:7.4", "GDPR:Art.25", "NIST:AC-2"}, controls)
	want := []string{"gdpr-25-1", "iso-a832", "pims-74", "soc-cc81"}

	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestCheckVerdict(t *testing.T) {
	cases := map[string]CheckReport{
		"pass":         {Status: "success", Passed: []Outcome{{Title: "ok"}}},
		"fail":         {Status: "failed", Findings: []Outcome{{Title: "bad"}}, Passed: []Outcome{{Title: "ok"}}},
		"inconclusive": {Status: "success"},
		"error":        {Status: "error", Findings: []Outcome{{Title: "bad"}}},
	}

	for want, c := range cases {
		if got := c.Verdict(); got != want {
			t.Errorf("verdict = %q, want %q", got, want)
		}
	}
}

func TestGithubSlug(t *testing.T) {
	for in, want := range map[string]string{
		"https://github.com/acme/api.git":                "acme/api",
		"git@github.com:acme/api.git":                    "acme/api",
		"ssh://git@github.com/acme/api":                  "acme/api",
		"http://local_proxy@127.0.0.1:1234/git/acme/api": "acme/api",
		"https://gitlab.com/acme/api.git":                "",
	} {
		if got := githubSlug(in); got != want {
			t.Errorf("githubSlug(%q) = %q, want %q", in, got, want)
		}
	}
}
