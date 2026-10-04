package guard

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// HookInput is the payload Claude Code (and compatible agents) send to a
// command hook on stdin.
type HookInput struct {
	SessionID      string          `json:"session_id"`
	TranscriptPath string          `json:"transcript_path,omitempty"`
	Cwd            string          `json:"cwd"`
	HookEventName  string          `json:"hook_event_name"`
	ToolName       string          `json:"tool_name,omitempty"`
	ToolInput      json.RawMessage `json:"tool_input,omitempty"`
	Prompt         string          `json:"prompt,omitempty"`
}

// Subject is the normalized view of a tool call that rules match against.
type Subject struct {
	Tool    string
	Path    string
	Command string
	Content string
}

func SubjectFromHook(in HookInput) Subject {
	s := Subject{Tool: in.ToolName}

	var ti struct {
		FilePath     string `json:"file_path"`
		NotebookPath string `json:"notebook_path"`
		Path         string `json:"path"`
		Command      string `json:"command"`
		Content      string `json:"content"`
		NewString    string `json:"new_string"`
		NewSource    string `json:"new_source"`
		Edits        []struct {
			NewString string `json:"new_string"`
		} `json:"edits"`
	}
	_ = json.Unmarshal(in.ToolInput, &ti)

	s.Path = firstNonEmpty(ti.FilePath, ti.NotebookPath, ti.Path)
	if s.Path != "" && in.Cwd != "" && !filepath.IsAbs(s.Path) {
		s.Path = filepath.Join(in.Cwd, s.Path)
	}

	s.Command = ti.Command

	parts := []string{ti.Content, ti.NewString, ti.NewSource}
	for _, e := range ti.Edits {
		parts = append(parts, e.NewString)
	}

	s.Content = strings.Join(nonEmpty(parts), "\n")

	return s
}

// Target is a short human-readable description of what the action touches.
func (s Subject) Target() string {
	if s.Path != "" {
		return s.Path
	}

	if s.Command != "" {
		return Truncate(s.Command, 200)
	}

	return ""
}

type Match struct {
	RuleID      string   `json:"rule_id"`
	Description string   `json:"description"`
	Action      Action   `json:"action"`
	Severity    string   `json:"severity,omitempty"`
	Controls    []string `json:"controls"`
}

type Decision struct {
	Action   Action   `json:"action"`
	Matches  []Match  `json:"matches"`
	Controls []string `json:"controls"`
	Reason   string   `json:"reason,omitempty"`
}

// Evaluate returns the strictest action of all matching rules. An action
// with no matching rule gets an empty Action and is not recorded.
func (p *Policy) Evaluate(s Subject) Decision {
	var d Decision

	controls := map[string]struct{}{}

	for i := range p.Rules {
		r := &p.Rules[i]
		if r.Disabled || !r.matches(s) {
			continue
		}

		d.Matches = append(d.Matches, Match{
			RuleID:      r.ID,
			Description: r.Description,
			Action:      r.Action,
			Severity:    r.Severity,
			Controls:    r.Controls,
		})

		if r.Action.rank() > d.Action.rank() {
			d.Action = r.Action
		}

		for _, c := range r.Controls {
			controls[c] = struct{}{}
		}
	}

	for c := range controls {
		d.Controls = append(d.Controls, c)
	}

	sort.Strings(d.Controls)

	var reasons []string
	for _, m := range d.Matches {
		if m.Action == d.Action {
			reasons = append(reasons, m.Description)
		}
	}

	if len(reasons) > 0 {
		d.Reason = "Black Fortress: " + strings.Join(reasons, "; ") + " (" + strings.Join(d.Controls, ", ") + ")"
	}

	return d
}

func (r *Rule) matches(s Subject) bool {
	if r.tools != nil && !r.tools.MatchString(s.Tool) {
		return false
	}

	if r.path != nil && !matchesPath(r, s) {
		return false
	}

	if r.command != nil && !r.command.MatchString(s.Command) {
		return false
	}

	if r.content != nil && !r.content.MatchString(s.Content) {
		return false
	}

	return true
}

// matchesPath tests the file path, or for shell commands every word that
// could be a path, so "cat .env", "cat<.env" and "cat .env|base64" are
// caught like Read(.env).
func matchesPath(r *Rule, s Subject) bool {
	if s.Path != "" {
		return r.path.MatchString(s.Path)
	}

	for _, field := range shellWordSplit.Split(s.Command, -1) {
		field = strings.TrimRight(strings.Trim(field, `"'`), "*?")
		if field != "" && r.path.MatchString(field) {
			return true
		}
	}

	return false
}

var shellWordSplit = regexp.MustCompile(`[\s|;&<>()]+`)

// Truncate shortens s to at most n bytes without splitting a UTF-8
// character, appending "…" when it cut anything. Invalid UTF-8 is repaired
// so the result survives a JSON round trip byte for byte.
func Truncate(s string, n int) string {
	s = strings.ToValidUTF8(s, "\uFFFD")
	if len(s) <= n {
		return s
	}

	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}

	return s[:cut] + "…"
}

func firstNonEmpty(v ...string) string {
	for _, x := range v {
		if x != "" {
			return x
		}
	}

	return ""
}

func nonEmpty(v []string) []string {
	out := v[:0]
	for _, x := range v {
		if x != "" {
			out = append(out, x)
		}
	}

	return out
}
