package cli

import (
	"strings"
	"testing"
)

func TestClaudeMCPArgsUseStdioBridge(t *testing.T) {
	args := claudeMCPArgs("/Applications/Black Fortress.app/bf")
	joined := strings.Join(args, " ")

	if strings.Contains(joined, "Bearer") || strings.Contains(joined, "--header") || strings.Contains(joined, "http") {
		t.Fatalf("registration must not carry the token or use HTTP: %q", joined)
	}

	n := len(args)
	if n < 3 || args[n-3] != "--" || args[n-2] != "/Applications/Black Fortress.app/bf" || args[n-1] != "mcp-stdio" {
		t.Fatalf("expected `-- <bf> mcp-stdio` at the end, got %q", args)
	}
}
