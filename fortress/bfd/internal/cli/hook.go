package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"blackfortress.dev/fortress/bfd/internal/guard"
	"blackfortress.dev/fortress/bfd/internal/paths"
	"blackfortress.dev/fortress/bfd/internal/secrets"
)

const sessionContext = `This machine is governed by Black Fortress, a local compliance platform.
Every file edit and shell command you make is evaluated against compliance controls
(SOC 2, ISO 27001, GDPR, HIPAA, PCI DSS and others) and recorded as audit evidence.
- Never write credentials, tokens or private keys into files; use environment variables or a secrets manager.
- Infrastructure, CI/CD, dependency, auth/crypto and personal-data schema changes are change-managed: explain the change and its risk in your summary.
- The black-fortress MCP server gives you the organization's frameworks, controls, measures, risks, policies and tasks. Check relevant controls before security-sensitive changes, and record evidence or update measure status when you implement a control.`

// maxHookInput bounds how much hook JSON is read from stdin.
var maxHookInput = 32 << 20

// Hook handles a Claude Code hook event. It must never break the agent: on
// any internal error it exits 0 with no output, except that block rules are
// evaluated locally first so enforcement does not depend on bfd running.
func Hook(args []string, stdin io.Reader, stdout io.Writer) (code int) {
	defer func() {
		if recover() != nil {
			code = 0
		}
	}()

	event := "PreToolUse"
	agent := "claude-code"

	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--agent" && i+1 < len(args):
			agent = args[i+1]
			i++
		case !strings.HasPrefix(a, "-"):
			event = a
		}
	}

	raw, err := io.ReadAll(io.LimitReader(stdin, int64(maxHookInput)+1))
	if err != nil {
		return 0
	}

	// Input past the limit cannot be parsed, so none of the rules could
	// inspect it. Fail closed for tool calls rather than let a large enough
	// write skip every guardrail.
	if len(raw) > maxHookInput {
		if event == "PreToolUse" {
			writeHookOutput(stdout, map[string]any{
				"hookSpecificOutput": map[string]any{
					"hookEventName":            "PreToolUse",
					"permissionDecision":       "ask",
					"permissionDecisionReason": "Black Fortress: this tool call is too large to check against the compliance guardrails; review it before approving.",
				},
			})
		}

		return 0
	}

	var in guard.HookInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return 0
	}

	if in.HookEventName != "" {
		event = in.HookEventName
	}

	home, err := paths.Home()
	if err != nil {
		return 0
	}

	layout := paths.NewLayout(home)
	sec, _ := secrets.Load(layout.Secrets)
	ledger := guard.Ledger{Dir: layout.Ledger, Key: sec.LedgerKeyBytes()}

	switch event {
	case "SessionStart":
		record(layout, ledger, &guard.Entry{Agent: agent, SessionID: in.SessionID, Cwd: in.Cwd, Repo: gitRoot(in.Cwd), Event: event})
		writeHookOutput(stdout, map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName":     "SessionStart",
				"additionalContext": sessionContext,
			},
		})

		return 0
	case "SessionEnd", "Stop":
		record(layout, ledger, &guard.Entry{Agent: agent, SessionID: in.SessionID, Cwd: in.Cwd, Repo: gitRoot(in.Cwd), Event: event})

		if dir := os.Getenv("BF_REPORT_DIR"); dir != "" && event == "SessionEnd" {
			writeSessionReport(dir, in.SessionID)
		}

		return 0
	case "PreToolUse", "PostToolUse":
	default:
		return 0
	}

	policy, err := guard.LoadPolicy(layout.Policy)
	if err != nil {
		// A broken user policy must not disable the built-in rules.
		if policy, err = guard.DefaultPolicy(); err != nil {
			return 0
		}
	}

	subject := guard.SubjectFromHook(in)
	decision := policy.Evaluate(subject)

	// Self-protection overrides any user policy.
	if p := guard.Protection(subject, home); p != nil {
		decision = *p
	}

	if decision.Action == "" {
		return 0
	}

	entry := &guard.Entry{
		Agent:     agent,
		SessionID: in.SessionID,
		Cwd:       in.Cwd,
		Repo:      gitRoot(in.Cwd),
		Event:     event,
		Tool:      subject.Tool,
		Target:    subject.Target(),
		Decision:  decision.Action,
		Controls:  decision.Controls,
		Reason:    decision.Reason,
	}
	for _, m := range decision.Matches {
		entry.Rules = append(entry.Rules, m.RuleID)
	}

	if event == "PostToolUse" {
		// The tool ran: for "ask" rules that means a human approved it.
		entry.Outcome = "executed"
		if decision.Action == guard.ActionAsk {
			entry.Outcome = "approved-and-executed"
		}

		record(layout, ledger, entry)

		return 0
	}

	switch decision.Action {
	case guard.ActionBlock:
		entry.Outcome = "blocked"
	case guard.ActionAsk:
		entry.Outcome = "awaiting-approval"
	default:
		entry.Outcome = "allowed"
	}

	record(layout, ledger, entry)

	switch decision.Action {
	case guard.ActionBlock:
		writeHookOutput(stdout, map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName":            "PreToolUse",
				"permissionDecision":       "deny",
				"permissionDecisionReason": decision.Reason,
			},
		})
	case guard.ActionAsk:
		writeHookOutput(stdout, map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName":            "PreToolUse",
				"permissionDecision":       "ask",
				"permissionDecisionReason": decision.Reason,
			},
		})
	}

	// "record" prints nothing so the agent's normal permission flow applies.
	return 0
}

func writeHookOutput(w io.Writer, v any) {
	_ = json.NewEncoder(w).Encode(v)
}

// record appends to the ledger and tells bfd (best effort) so the app
// updates live. The ledger write happens here, not in bfd, so evidence is
// kept even while bfd is stopped.
func record(layout paths.Layout, ledger guard.Ledger, e *guard.Entry) {
	if err := ledger.Append(e); err != nil {
		fmt.Fprintln(os.Stderr, "black fortress: cannot write ledger:", err)
		return
	}

	sec, err := secrets.Load(layout.Secrets)
	if err != nil || sec.MCPToken == "" {
		return
	}

	body, _ := json.Marshal(e)

	req, err := http.NewRequest(http.MethodPost, controlURL()+"/v1/hooks/"+e.Event, bytes.NewReader(body))
	if err != nil {
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sec.MCPToken)

	client := &http.Client{Timeout: 750 * time.Millisecond}
	if resp, err := client.Do(req); err == nil {
		resp.Body.Close()
	}
}

func controlURL() string {
	port := paths.ControlPort
	if v, err := strconv.Atoi(os.Getenv("BF_CONTROL_PORT")); err == nil && v > 0 {
		port = v
	}

	return "http://127.0.0.1:" + strconv.Itoa(port)
}

func gitRoot(dir string) string {
	if dir == "" {
		return ""
	}

	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return ""
	}

	return filepath.Clean(strings.TrimSpace(string(out)))
}
