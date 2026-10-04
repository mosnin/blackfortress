package guard

import (
	"path/filepath"
	"regexp"
	"strings"
)

// Self-protection keeps the governed agent from switching off its own
// governance: reading Black Fortress secrets (which would let it forge
// ledger entries), changing the policy or ledger, or removing its hooks
// from Claude Code settings. These rules are built in and cannot be
// overridden by policy.json.

var protectionControls = []string{"SOC2:CC6.1", "SOC2:CC7.2", "ISO27001:A.8.15", "ISO27001:A.5.15"}

var claudeSettingsRe = regexp.MustCompile(`(^|/)\.claude/settings(\.local)?\.json$`)

// Protection evaluates s against the built-in self-protection rules for an
// installation whose data directory is home. It returns nil when no rule
// applies.
func Protection(s Subject, home string) *Decision {
	if home == "" {
		return nil
	}

	home = filepath.Clean(home)
	secretsPath := filepath.Join(home, "secrets.json")

	block := func(reason string) *Decision {
		return &Decision{
			Action:   ActionBlock,
			Controls: protectionControls,
			Reason:   "Black Fortress: " + reason,
			Matches:  []Match{{RuleID: "fortress.self-protection", Description: reason, Action: ActionBlock, Severity: "high", Controls: protectionControls}},
		}
	}

	switch s.Tool {
	case "Read", "Grep", "Glob":
		if s.Path != "" && (filepath.Clean(s.Path) == secretsPath || strings.HasPrefix(filepath.Clean(s.Path), secretsPath)) {
			return block("agents may not read Black Fortress secrets")
		}
	case "Write", "Edit", "MultiEdit", "NotebookEdit":
		p := filepath.Clean(s.Path)
		if p == home || strings.HasPrefix(p, home+string(filepath.Separator)) {
			return block("agents may not modify Black Fortress data, policy or ledger")
		}

		if claudeSettingsRe.MatchString(filepath.ToSlash(p)) {
			return block("agents may not change Claude Code settings, which hold the compliance hooks")
		}
	case "Bash":
		cmd := normalizeShell(s.Command)
		if strings.Contains(cmd, normalizeShell(home)) || strings.Contains(cmd, normalizeShell(homeSuffix(home))) {
			return block("agents may not access the Black Fortress data directory from the shell")
		}

		if strings.Contains(cmd, ".claude/settings") && writesFiles(cmd) {
			return block("agents may not change Claude Code settings, which hold the compliance hooks")
		}
	}

	return nil
}

// homeSuffix is the platform-specific tail of the data directory
// ("Application Support/BlackFortress" or ".local/share/blackfortress"), so
// "~/..." and "$HOME/..." spellings are caught too. A bare "BlackFortress"
// would also match the user's own repositories, so it is not used.
func homeSuffix(home string) string {
	parent := filepath.Base(filepath.Dir(home))
	return parent + "/" + filepath.Base(home)
}

// normalizeShell drops quoting and escapes so `Application\ Support` and
// "Application Support" compare equal.
func normalizeShell(s string) string {
	return strings.NewReplacer(`\ `, " ", `"`, "", `'`, "").Replace(s)
}

var writeCommandRe = regexp.MustCompile(`(>|\btee\b|\bsed\s+-i|\bperl\s+-i|\bmv\b|\bcp\b|\brm\b|\bjq\b.*>|\btruncate\b|\bchmod\b|\bln\b|\bpython3?\b|\bnode\b|\bruby\b)`)

func writesFiles(cmd string) bool {
	return writeCommandRe.MatchString(cmd)
}
