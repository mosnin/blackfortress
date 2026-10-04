package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestHookAsksWhenInputTooLargeToInspect(t *testing.T) {
	old := maxHookInput
	maxHookInput = 64
	t.Cleanup(func() { maxHookInput = old })

	big := `{"hook_event_name":"PreToolUse","tool_name":"Write","tool_input":{"content":"` + strings.Repeat("x", 200) + `"}}`

	var out bytes.Buffer
	if code := Hook([]string{"PreToolUse"}, strings.NewReader(big), &out); code != 0 {
		t.Fatalf("exit code %d", code)
	}

	if !strings.Contains(out.String(), `"permissionDecision":"ask"`) {
		t.Fatalf("oversized PreToolUse input must ask, got %q", out.String())
	}

	out.Reset()
	Hook([]string{"PostToolUse"}, strings.NewReader(big), &out)

	if out.Len() != 0 {
		t.Fatalf("oversized PostToolUse input must stay silent, got %q", out.String())
	}
}
