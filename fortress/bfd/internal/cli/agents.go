package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"blackfortress.dev/fortress/bfd/internal/paths"
	"blackfortress.dev/fortress/bfd/internal/secrets"
)

const mcpServerName = "black-fortress"

// hookEvents are the Claude Code events bf handles, with their matchers.
var hookEvents = []struct {
	Event   string
	Matcher string
}{
	{"PreToolUse", "Write|Edit|MultiEdit|NotebookEdit|Bash|Read"},
	{"PostToolUse", "Write|Edit|MultiEdit|NotebookEdit|Bash"},
	{"SessionStart", ""},
	{"SessionEnd", ""},
}

func bfPath() string {
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}

		if filepath.Base(exe) == "bf" {
			return exe
		}

		if sibling := filepath.Join(filepath.Dir(exe), "bf"); fileExists(sibling) {
			return sibling
		}
	}

	if p, err := exec.LookPath("bf"); err == nil {
		return p
	}

	return "bf"
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func loadSecrets() (*secrets.Secrets, error) {
	home, err := paths.Home()
	if err != nil {
		return nil, err
	}

	sec, err := secrets.Load(paths.NewLayout(home).Secrets)
	if err != nil {
		return nil, errors.New("Black Fortress is not set up yet: start the app (or `bfd run`) once first")
	}

	return sec, nil
}

// AgentConfig prints MCP (and for Claude Code, hook) configuration.
func AgentConfig(args []string, stdout io.Writer) int {
	agent := "claude"
	if len(args) > 0 {
		agent = args[0]
	}

	bf := bfPath()

	switch agent {
	case "claude":
		fmt.Fprintf(stdout, "# MCP server (user scope):\nclaude %s\n\n", strings.Join(quoteAll(claudeMCPArgs(bf)), " "))
		fmt.Fprintln(stdout, "# Hooks (merge into ~/.claude/settings.json, or run `bf install-claude`):")
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{"hooks": claudeHooks(bf)})
	case "cursor":
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		fmt.Fprintln(stdout, "# ~/.cursor/mcp.json")
		_ = enc.Encode(map[string]any{"mcpServers": map[string]any{
			mcpServerName: map[string]any{"command": bf, "args": []string{"mcp-stdio"}},
		}})
	case "http":
		// For clients without stdio support. This prints the bearer token,
		// which then sits in the client's config where agents can read it.
		sec, err := loadSecrets()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}

		url := strings.Replace(strings.TrimSuffix(controlURL(), "/")+"/mcp", "127.0.0.1", "localhost", 1)
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{"mcpServers": map[string]any{
			mcpServerName: map[string]any{"url": url, "headers": map[string]string{"Authorization": "Bearer " + sec.MCPToken}},
		}})
	case "codex":
		fmt.Fprintf(stdout, "# ~/.codex/config.toml\n[mcp_servers.%s]\ncommand = %q\nargs = [\"mcp-stdio\"]\n", strings.ReplaceAll(mcpServerName, "-", "_"), bf)
	case "stdio":
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{"mcpServers": map[string]any{
			mcpServerName: map[string]any{"command": bf, "args": []string{"mcp-stdio"}},
		}})
	default:
		fmt.Fprintf(os.Stderr, "unknown agent %q (claude, cursor, codex, stdio, http)\n", agent)
		return 2
	}

	return 0
}

func claudeHooks(bf string) map[string]any {
	hooks := map[string]any{}

	for _, h := range hookEvents {
		group := map[string]any{
			"hooks": []map[string]any{{
				"type":    "command",
				"command": shellQuote(bf) + " hook " + h.Event,
				"timeout": 10,
			}},
		}
		if h.Matcher != "" {
			group["matcher"] = h.Matcher
		}

		hooks[h.Event] = []any{group}
	}

	return hooks
}

func shellQuote(s string) string {
	if !strings.ContainsAny(s, " '\"$`\\") {
		return s
	}

	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// InstallClaude merges bf's hooks into a Claude Code settings file and
// registers the MCP server with the `claude` CLI when it is installed.
func InstallClaude(args []string, stdout io.Writer) int {
	fs := flag.NewFlagSet("install-claude", flag.ContinueOnError)
	project := fs.String("project", "", "install into DIR/.claude/settings.json instead of ~/.claude/settings.json")
	skipMCP := fs.Bool("skip-mcp", false, "only install hooks")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	settingsPath := ""
	if *project != "" {
		settingsPath = filepath.Join(*project, ".claude", "settings.json")
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}

		settingsPath = filepath.Join(home, ".claude", "settings.json")
	}

	if err := mergeClaudeHooks(settingsPath, bfPath()); err != nil {
		fmt.Fprintln(os.Stderr, "cannot update", settingsPath+":", err)
		return 1
	}

	fmt.Fprintln(stdout, "✓ hooks installed in", settingsPath)

	if *skipMCP {
		return 0
	}

	mcpArgs := claudeMCPArgs(bfPath())

	claude, err := exec.LookPath("claude")
	if err != nil {
		fmt.Fprintln(stdout, "! `claude` CLI not found; register the MCP server with:")
		fmt.Fprintf(stdout, "  claude %s\n", strings.Join(quoteAll(mcpArgs), " "))

		return 0
	}

	// Re-adding an existing server fails, so remove any stale entry first.
	_ = exec.Command(claude, "mcp", "remove", "--scope", "user", mcpServerName).Run()

	if out, err := exec.Command(claude, mcpArgs...).CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "claude mcp add failed: %v\n%s", err, out)
		return 1
	}

	fmt.Fprintln(stdout, "✓ MCP server", mcpServerName, "registered with Claude Code")

	return 0
}

// claudeMCPArgs registers bfd with Claude Code through the `bf mcp-stdio`
// bridge, which reads the bearer token from secrets.json itself. The token
// never appears on a command line (visible in ps) or in ~/.claude.json,
// where the governed agent could read it.
func claudeMCPArgs(bf string) []string {
	return []string{"mcp", "add", "--scope", "user", mcpServerName, "--", bf, "mcp-stdio"}
}

func quoteAll(v []string) []string {
	out := make([]string, len(v))
	for i, s := range v {
		out[i] = shellQuote(s)
	}

	return out
}

// mergeClaudeHooks adds bf's hook groups, replacing earlier bf entries and
// keeping everything else in the file untouched.
func mergeClaudeHooks(path, bf string) error {
	settings := map[string]any{}

	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}
	case errors.Is(err, os.ErrNotExist):
	default:
		return err
	}

	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}

	for event, groups := range claudeHooks(bf) {
		existing, _ := hooks[event].([]any)

		kept := make([]any, 0, len(existing)+1)
		for _, g := range existing {
			if !isBFGroup(g) {
				kept = append(kept, g)
			}
		}

		hooks[event] = append(kept, groups.([]any)...)
	}

	settings["hooks"] = hooks

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	if len(data) > 0 {
		if err := os.WriteFile(path+".bak", data, 0o600); err != nil {
			return err
		}
	}

	return os.WriteFile(path, append(out, '\n'), 0o600)
}

func isBFGroup(g any) bool {
	m, ok := g.(map[string]any)
	if !ok {
		return false
	}

	list, _ := m["hooks"].([]any)
	for _, h := range list {
		hm, _ := h.(map[string]any)
		cmd, _ := hm["command"].(string)

		if strings.Contains(cmd, " hook ") && (strings.Contains(cmd, "/bf") || strings.HasPrefix(cmd, "bf ")) {
			return true
		}
	}

	return false
}
