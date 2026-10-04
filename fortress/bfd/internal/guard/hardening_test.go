package guard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDestructiveVariants(t *testing.T) {
	p, _ := DefaultPolicy()

	for _, cmd := range []string{
		"rm -rf /*", "rm -fr /", "rm -r -f /", "rm -Rf /", "rm -rf ~/", "rm -rf $HOME/",
		"rm --recursive --force /", "psql -c 'drop table users'", "git push origin +main",
		"git push --force-with-lease origin main", "rm -rf ~ && echo done",
	} {
		d := p.Evaluate(SubjectFromHook(hook("Bash", map[string]any{"command": cmd})))
		if d.Action != ActionAsk {
			t.Errorf("%q: action %q, want ask", cmd, d.Action)
		}
	}

	for _, cmd := range []string{"rm -rf ./build", "rm -rf node_modules", "git push origin feature/x", "echo drop"} {
		if d := p.Evaluate(SubjectFromHook(hook("Bash", map[string]any{"command": cmd}))); d.Action != "" {
			t.Errorf("%q matched %+v", cmd, d.Matches)
		}
	}
}

func TestSecretReadVariants(t *testing.T) {
	p, _ := DefaultPolicy()

	for _, cmd := range []string{"cat .env|base64", "cat<.env", "cat ./.env;", "head .env*", "(cat .env.local)"} {
		d := p.Evaluate(SubjectFromHook(hook("Bash", map[string]any{"command": cmd})))
		if d.Action != ActionAsk {
			t.Errorf("%q: action %q, want ask", cmd, d.Action)
		}
	}
}

func TestProtection(t *testing.T) {
	home := "/Users/dev/Library/Application Support/BlackFortress"

	blocked := []Subject{
		{Tool: "Read", Path: home + "/secrets.json"},
		{Tool: "Write", Path: home + "/policy.json"},
		{Tool: "Edit", Path: home + "/ledger/2026-10-04.jsonl"},
		{Tool: "Edit", Path: "/Users/dev/.claude/settings.json"},
		{Tool: "Write", Path: "/repo/.claude/settings.local.json"},
		{Tool: "Bash", Command: `cat ~/Library/Application\ Support/BlackFortress/secrets.json`},
		{Tool: "Bash", Command: `sed -i '' s/block/record/ "$HOME/Library/Application Support/BlackFortress/policy.json"`},
		{Tool: "Bash", Command: `jq 'del(.hooks)' ~/.claude/settings.json > /tmp/x && mv /tmp/x ~/.claude/settings.json`},
	}
	for _, s := range blocked {
		if d := Protection(s, home); d == nil || d.Action != ActionBlock {
			t.Errorf("%+v not blocked", s)
		}
	}

	allowed := []Subject{
		{Tool: "Read", Path: home + "/ledger/2026-10-04.jsonl"},
		{Tool: "Edit", Path: "/Users/dev/code/blackfortress/README.md"},
		{Tool: "Bash", Command: "cd ~/code/blackfortress && go test ./..."},
		{Tool: "Bash", Command: "cat ~/.claude/settings.json"},
	}
	for _, s := range allowed {
		if d := Protection(s, home); d != nil {
			t.Errorf("%+v blocked: %s", s, d.Reason)
		}
	}
}

func TestTruncateKeepsChainValid(t *testing.T) {
	l := Ledger{Dir: t.TempDir(), Key: []byte("k")}

	// Byte 200 falls inside a multi-byte character.
	cmd := strings.Repeat("a", 199) + "—tail"
	s := Subject{Tool: "Bash", Command: cmd}

	if err := l.Append(&Entry{Agent: "t", Event: "PreToolUse", Target: s.Target()}); err != nil {
		t.Fatal(err)
	}

	if err := l.Append(&Entry{Agent: "t", Event: "x", Target: "bad \xff utf8"}); err != nil {
		t.Fatal(err)
	}

	if n, err := l.Verify(); err != nil || n != 2 {
		t.Fatalf("verify = %d, %v", n, err)
	}
}

func TestLedgerMACRejectsForgery(t *testing.T) {
	dir := t.TempDir()
	signed := Ledger{Dir: dir, Key: []byte("secret")}

	if err := signed.Append(&Entry{Agent: "t", Event: "x"}); err != nil {
		t.Fatal(err)
	}

	// An attacker without the key rewrites the day with a valid hash chain.
	files, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	_ = os.Remove(files[0])

	forger := Ledger{Dir: dir}
	if err := forger.Append(&Entry{Agent: "t", Event: "forged"}); err != nil {
		t.Fatal(err)
	}

	if _, err := (Ledger{Dir: dir}).Verify(); err != nil {
		t.Fatalf("unkeyed verify should still pass the plain chain: %v", err)
	}

	if _, err := signed.Verify(); err == nil {
		t.Fatal("forged entry accepted")
	}
}

func TestLedgerSurvivesTornAndCorruptLines(t *testing.T) {
	l := Ledger{Dir: t.TempDir()}

	if err := l.Append(&Entry{Agent: "t", Event: "one"}); err != nil {
		t.Fatal(err)
	}

	files, _ := filepath.Glob(filepath.Join(l.Dir, "*.jsonl"))

	f, _ := os.OpenFile(files[0], os.O_APPEND|os.O_WRONLY, 0o600)
	_, _ = f.WriteString(`{"agent":"t","event":"torn`)
	f.Close()

	if err := l.Append(&Entry{Agent: "t", Event: "two"}); err != nil {
		t.Fatalf("append after torn line: %v", err)
	}

	recent, err := l.Recent(10)
	if err != nil || len(recent) != 2 || recent[0].Event != "two" {
		t.Fatalf("recent = %+v, %v", recent, err)
	}

	if _, err := l.Verify(); err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("verify should report the torn line, got %v", err)
	}
}
