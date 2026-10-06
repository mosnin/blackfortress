package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

const (
	maxFlaggedRows  = 50
	maxFindingRows  = 50
	maxControlsRows = 15
)

var verdictTitle = map[string]string{
	VerdictPass:      "✅ PASS",
	VerdictAttention: "⚠️ NEEDS ATTENTION",
	VerdictFail:      "❌ FAIL",
}

// RenderMarkdown writes the report for a human reviewer, for example as a
// pull request comment or a CI job summary.
func RenderMarkdown(w io.Writer, r *Report) error {
	b := bufio.NewWriter(w)
	p := func(format string, a ...any) { fmt.Fprintf(b, format, a...) }

	p("# Black Fortress compliance report\n\n")
	p("**Verdict: %s**\n\n", verdictTitle[r.Verdict])

	for _, reason := range r.Reasons {
		p("- %s\n", cell(reason, 300))
	}

	if len(r.Reasons) > 0 {
		p("\n")
	}

	p("| | |\n|---|---|\n")
	p("| Host | %s |\n", cell(r.Host, 80))
	p("| Window | %s → %s (UTC) |\n", r.Since.Format("2006-01-02 15:04"), r.GeneratedAt.Format("2006-01-02 15:04"))

	if r.Session != "" {
		p("| Session | `%s` |\n", cell(r.Session, 80))
	}

	if r.Runtime != nil {
		p("| Runtime | %s, version %s |\n", r.Runtime.State, cell(r.Runtime.Version, 40))
	} else {
		p("| Runtime | not running |\n")
	}

	renderActivity(p, r)
	renderFlagged(p, r)
	renderChecks(p, r)
	renderPosture(p, r)

	p("\n## Evidence integrity\n\n")

	if r.Ledger.Valid {
		p("The hash-chained, HMAC-signed evidence ledger verified: %d entries intact.\n", r.Ledger.Entries)
	} else {
		p("**The evidence ledger failed verification** after %d valid entries: %s\n", r.Ledger.Entries, cell(r.Ledger.Error, 300))
	}

	return b.Flush()
}

type printer func(format string, a ...any)

func renderActivity(p printer, r *Report) {
	a := r.Activity

	p("\n## Agent activity\n\n")
	p("%d tool calls evaluated: %d blocked, %d needed approval, %d recorded as change evidence.\n\n", a.Actions, a.Blocked, a.Asked, a.Recorded)

	if len(a.Sessions) > 0 {
		p("- Sessions: %s\n", cell(strings.Join(a.Sessions, ", "), 600))
	}

	if len(a.Repos) > 0 {
		p("- Repositories: %s\n", cell(strings.Join(a.Repos, ", "), 600))
	}

	if len(a.Controls) == 0 {
		return
	}

	p("\nControls this activity produced evidence for (most first):\n\n| Control | Actions |\n|---|---|\n")

	controls := sortedKeys(a.Controls)
	sortByCount(controls, a.Controls)

	for i, c := range controls {
		if i == maxControlsRows {
			p("| … %d more | |\n", len(controls)-maxControlsRows)
			break
		}

		p("| %s | %d |\n", cell(c, 60), a.Controls[c])
	}
}

func renderFlagged(p printer, r *Report) {
	if len(r.Flagged) == 0 {
		return
	}

	p("\n## Guardrail interventions\n\n| Time (UTC) | Decision | Tool | Target | Rule | Reason |\n|---|---|---|---|---|---|\n")

	for i, e := range r.Flagged {
		if i == maxFlaggedRows {
			p("\n… and %d more; see `bf report --format json`.\n", len(r.Flagged)-maxFlaggedRows)
			break
		}

		p("| %s | %s | %s | `%s` | %s | %s |\n",
			e.Time.UTC().Format("01-02 15:04:05"), e.Decision, cell(e.Tool, 30), cell(e.Target, 80),
			cell(strings.Join(e.Rules, ", "), 60), cell(e.Reason, 160))
	}
}

func renderChecks(p printer, r *Report) {
	p("\n## Automated checks\n\n")

	if r.Checks == nil || (len(r.Checks.Providers) == 0 && len(r.Checks.Skipped) == 0) {
		p("No automated check results yet.\n")
		return
	}

	p("| Provider | Pass | Fail | Inconclusive | Error | Ran (UTC) |\n|---|---|---|---|---|---|\n")

	type finding struct{ provider, check, title, resource, severity, fix string }
	var findings []finding

	for _, name := range sortedKeys(r.Checks.Providers) {
		run := r.Checks.Providers[name]
		if run.Error != "" {
			p("| %s | | | | %s | %s |\n", cell(run.ProviderName, 40), cell(run.Error, 120), run.RanAt.UTC().Format("01-02 15:04"))
			continue
		}

		counts := map[string]int{}
		for _, c := range run.Checks {
			counts[c.Verdict()]++

			for _, f := range c.Findings {
				findings = append(findings, finding{run.ProviderName, c.Name, f.Title, f.ResourceID, f.Severity, f.Remediation})
			}
		}

		p("| %s | %d | %d | %d | %d | %s |\n", cell(run.ProviderName, 40), counts["pass"], counts["fail"], counts["inconclusive"], counts["error"], run.RanAt.UTC().Format("01-02 15:04"))
	}

	for _, name := range sortedKeys(r.Checks.Skipped) {
		p("| %s | | | | skipped: %s | |\n", cell(name, 40), cell(r.Checks.Skipped[name], 120))
	}

	if len(findings) == 0 {
		return
	}

	p("\n### Findings\n\n| Provider | Check | Finding | Resource | Severity | Remediation |\n|---|---|---|---|---|---|\n")

	for i, f := range findings {
		if i == maxFindingRows {
			p("\n… and %d more; see `bf report --format json`.\n", len(findings)-maxFindingRows)
			break
		}

		p("| %s | %s | %s | `%s` | %s | %s |\n", cell(f.provider, 30), cell(f.check, 60), cell(f.title, 120), cell(f.resource, 80), cell(f.severity, 12), cell(f.fix, 200))
	}
}

func renderPosture(p printer, r *Report) {
	if r.Posture == nil || len(r.Posture.Frameworks) == 0 {
		return
	}

	p("\n## Framework posture\n\n| Framework | Controls | Implemented | In progress | Not started | No measure | Score |\n|---|---|---|---|---|---|---|\n")

	for _, f := range r.Posture.Frameworks {
		p("| %s | %d | %d | %d | %d | %d | %.0f%% |\n", cell(f.Name, 60), f.Controls, f.ControlsImplemented, f.ControlsInProgress, f.ControlsNotStarted, f.ControlsWithoutMeasure, f.Score*100)
	}
}

// cell makes text safe for a single Markdown table cell.
func cell(s string, n int) string {
	s = oneLine(s, n)
	s = strings.ReplaceAll(s, "|", "\\|")

	return strings.ReplaceAll(s, "`", "'")
}

func sortByCount(keys []string, counts map[string]int) {
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && counts[keys[j]] > counts[keys[j-1]]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
}
