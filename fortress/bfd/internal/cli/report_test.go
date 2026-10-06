package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"blackfortress.dev/fortress/bfd/internal/guard"
	"blackfortress.dev/fortress/bfd/internal/runtime"
)

func TestParseSince(t *testing.T) {
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

	if got, _ := parseSince("90m", now); !got.Equal(now.Add(-90 * time.Minute)) {
		t.Errorf("90m: %v", got)
	}

	if got, _ := parseSince("2026-10-01T00:00:00Z", now); !got.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("RFC 3339: %v", got)
	}

	if _, err := parseSince("yesterday", now); err == nil {
		t.Error("expected an error for an unparseable --since")
	}
}

func TestFailsOn(t *testing.T) {
	cases := []struct {
		verdict, threshold string
		want               bool
	}{
		{VerdictFail, "never", false},
		{VerdictFail, VerdictFail, true},
		{VerdictAttention, VerdictFail, false},
		{VerdictAttention, VerdictAttention, true},
		{VerdictPass, VerdictAttention, false},
	}

	for _, c := range cases {
		if got := failsOn(c.verdict, c.threshold); got != c.want {
			t.Errorf("failsOn(%s, %s) = %v", c.verdict, c.threshold, got)
		}
	}
}

func TestJudge(t *testing.T) {
	running := &runtime.Status{State: "running"}
	failing := &runtime.ChecksState{Providers: map[string]*runtime.ProviderRun{
		"github": {ProviderName: "GitHub", Checks: []runtime.CheckReport{{Name: "Branch protection", Findings: []runtime.Outcome{{Title: "main is unprotected"}}}}},
	}}

	cases := []struct {
		name string
		r    Report
		want string
	}{
		{"clean", Report{Runtime: running, Ledger: LedgerIntegrity{Valid: true}}, VerdictPass},
		{"runtime down", Report{Ledger: LedgerIntegrity{Valid: true}}, VerdictAttention},
		{"blocked action", Report{Runtime: running, Ledger: LedgerIntegrity{Valid: true}, Activity: Activity{Blocked: 1}}, VerdictAttention},
		{"failing check", Report{Runtime: running, Ledger: LedgerIntegrity{Valid: true}, Checks: failing}, VerdictFail},
		{"tampered ledger", Report{Runtime: running, Ledger: LedgerIntegrity{Error: "hash mismatch"}}, VerdictFail},
	}

	for _, c := range cases {
		got, reasons := judge(&c.r)
		if got != c.want {
			t.Errorf("%s: verdict %s, want %s (%v)", c.name, got, c.want, reasons)
		}

		if got != VerdictPass && len(reasons) == 0 {
			t.Errorf("%s: no reasons given for %s", c.name, got)
		}
	}
}

// BuildReport works from the ledger alone when bfd is not running, and
// honours the time window and session filter.
func TestBuildReportFromLedger(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BF_HOME", home)
	t.Setenv("BF_CONTROL_PORT", "1") // nothing listens there

	l := guard.Ledger{Dir: filepath.Join(home, "ledger")}
	if err := os.MkdirAll(l.Dir, 0o700); err != nil {
		t.Fatal(err)
	}

	for _, e := range []guard.Entry{
		{Agent: "claude-code", SessionID: "s1", Event: "PreToolUse", Tool: "Write", Target: "a.ts", Decision: guard.ActionBlock, Rules: []string{"secrets.hardcoded"}, Controls: []string{"SOC2:CC6.1"}, Reason: "credential | in file"},
		{Agent: "claude-code", SessionID: "s1", Event: "PreToolUse", Tool: "Edit", Target: "main.tf", Decision: guard.ActionRecord, Controls: []string{"SOC2:CC8.1"}},
		{Agent: "claude-code", SessionID: "s2", Event: "PreToolUse", Tool: "Bash", Target: "git push --force", Decision: guard.ActionAsk},
	} {
		e := e
		if err := l.Append(&e); err != nil {
			t.Fatal(err)
		}
	}

	r, err := BuildReport(time.Now().Add(-time.Hour), "s1")
	if err != nil {
		t.Fatal(err)
	}

	if r.Activity.Actions != 2 || r.Activity.Blocked != 1 || r.Activity.Recorded != 1 || r.Activity.Asked != 0 {
		t.Errorf("session filter: %+v", r.Activity)
	}

	if !r.Ledger.Valid || r.Ledger.Entries != 3 {
		t.Errorf("ledger: %+v", r.Ledger)
	}

	if r.Runtime != nil || r.Verdict != VerdictAttention {
		t.Errorf("runtime %v verdict %s", r.Runtime, r.Verdict)
	}

	var md bytes.Buffer
	if err := RenderMarkdown(&md, r); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"NEEDS ATTENTION", "Guardrail interventions", "secrets.hardcoded", `credential \| in file`, "3 entries intact"} {
		if !strings.Contains(md.String(), want) {
			t.Errorf("markdown lacks %q:\n%s", want, md.String())
		}
	}

	if r, _ := BuildReport(time.Now().Add(time.Minute), ""); r.Activity.Actions != 0 {
		t.Errorf("window in the future still counted %d actions", r.Activity.Actions)
	}
}

func TestSessionReportFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BF_HOME", home)
	t.Setenv("BF_CONTROL_PORT", "1")

	out := filepath.Join(t.TempDir(), "reports")
	writeSessionReport(out, "abc/../def")

	data, err := os.ReadFile(filepath.Join(out, "blackfortress-abc_.._def.json"))
	if err != nil {
		t.Fatal(err)
	}

	var r Report
	if err := json.Unmarshal(data, &r); err != nil || r.Session != "abc/../def" {
		t.Fatalf("session report: %v %+v", err, r)
	}

	if _, err := os.Stat(filepath.Join(out, "blackfortress-abc_.._def.md")); err != nil {
		t.Fatal(err)
	}
}
