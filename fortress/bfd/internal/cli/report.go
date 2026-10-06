package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"blackfortress.dev/fortress/bfd/internal/guard"
	"blackfortress.dev/fortress/bfd/internal/paths"
	"blackfortress.dev/fortress/bfd/internal/runtime"
	"blackfortress.dev/fortress/bfd/internal/secrets"
)

// Report is what `bf report` produces: everything a reviewer needs to judge
// an agent session (or a period of work) without access to the machine.
type Report struct {
	GeneratedAt time.Time            `json:"generated_at"`
	Host        string               `json:"host"`
	Since       time.Time            `json:"since"`
	Session     string               `json:"session,omitempty"`
	Verdict     string               `json:"verdict"`
	Reasons     []string             `json:"reasons"`
	Runtime     *runtime.Status      `json:"runtime,omitempty"`
	Posture     *runtime.Posture     `json:"posture,omitempty"`
	Checks      *runtime.ChecksState `json:"checks,omitempty"`
	Activity    Activity             `json:"activity"`
	Flagged     []guard.Entry        `json:"flagged"`
	Ledger      LedgerIntegrity      `json:"ledger"`
}

// Activity counts what agents did in the report window.
type Activity struct {
	Actions  int            `json:"actions"`
	Blocked  int            `json:"blocked"`
	Asked    int            `json:"asked"`
	Recorded int            `json:"recorded"`
	Sessions []string       `json:"sessions"`
	Repos    []string       `json:"repos"`
	Controls map[string]int `json:"controls"`
}

type LedgerIntegrity struct {
	Valid   bool   `json:"valid"`
	Entries int    `json:"entries"`
	Error   string `json:"error,omitempty"`
}

const (
	VerdictPass      = "pass"
	VerdictAttention = "attention"
	VerdictFail      = "fail"
)

// ReportCmd implements `bf report`.
func ReportCmd(args []string, stdout io.Writer) int {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	since := fs.String("since", "24h", "window start: a duration back from now (24h, 90m) or an RFC 3339 time")
	session := fs.String("session", "", "only this agent session id")
	format := fs.String("format", "md", "md or json")
	out := fs.String("out", "", "write to this file instead of stdout")
	failOn := fs.String("fail-on", "never", "exit 1 when the verdict is at least: fail, attention or never")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	start, err := parseSince(*since, time.Now())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	r, err := BuildReport(start, *session)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	w := stdout
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer f.Close()
		w = f
	}

	switch *format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		err = enc.Encode(r)
	case "md", "markdown":
		err = RenderMarkdown(w, r)
	default:
		fmt.Fprintf(os.Stderr, "unknown format %q (md, json)\n", *format)
		return 2
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if failsOn(r.Verdict, *failOn) {
		return 1
	}

	return 0
}

// writeSessionReport saves the report of one finished agent session as
// Markdown and JSON in dir (BF_REPORT_DIR), for headless machines whose
// operator collects reports instead of watching the app.
func writeSessionReport(dir, session string) {
	if session == "" {
		return
	}

	r, err := BuildReport(time.Now().Add(-30*24*time.Hour), session)
	if err != nil {
		fmt.Fprintln(os.Stderr, "black fortress: cannot build session report:", err)
		return
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "black fortress:", err)
		return
	}

	base := filepath.Join(dir, "blackfortress-"+safeName(session))

	if f, err := os.Create(base + ".md"); err == nil {
		_ = RenderMarkdown(f, r)
		f.Close()
	}

	if data, err := json.MarshalIndent(r, "", "  "); err == nil {
		_ = os.WriteFile(base+".json", append(data, '\n'), 0o644)
	}
}

func safeName(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '-' || r == '_' || r == '.' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}

		return '_'
	}, s)
}

func parseSince(v string, now time.Time) (time.Time, error) {
	if d, err := time.ParseDuration(v); err == nil {
		return now.Add(-d), nil
	}

	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, nil
	}

	return time.Time{}, fmt.Errorf("--since %q: want a duration (24h) or an RFC 3339 time", v)
}

func failsOn(verdict, threshold string) bool {
	switch threshold {
	case VerdictFail:
		return verdict == VerdictFail
	case VerdictAttention:
		return verdict != VerdictPass
	default:
		return false
	}
}

// BuildReport gathers the report from the ledger on disk and, when it is
// running, from bfd. It works with bfd stopped: the ledger alone still
// shows what the agents did.
func BuildReport(since time.Time, session string) (*Report, error) {
	home, err := paths.Home()
	if err != nil {
		return nil, err
	}

	layout := paths.NewLayout(home)
	sec, _ := secrets.Load(layout.Secrets)
	ledger := guard.Ledger{Dir: layout.Ledger, Key: sec.LedgerKeyBytes()}

	host, _ := os.Hostname()
	r := &Report{GeneratedAt: time.Now().UTC(), Host: host, Since: since.UTC(), Session: session, Flagged: []guard.Entry{}}

	entries, err := entriesSince(ledger, since, session)
	if err != nil {
		return nil, err
	}

	r.Activity, r.Flagged = summarize(entries)

	n, err := ledger.Verify()
	r.Ledger = LedgerIntegrity{Valid: err == nil, Entries: n}
	if err != nil {
		r.Ledger.Error = err.Error()
	}

	if sec != nil {
		c := &apiClient{token: sec.MCPToken}
		r.Runtime = getJSON[runtime.Status](c, "/v1/status")
		r.Posture = getJSON[runtime.Posture](c, "/v1/posture")
		r.Checks = getJSON[runtime.ChecksState](c, "/v1/checks?detail=1")
	}

	r.Verdict, r.Reasons = judge(r)

	return r, nil
}
