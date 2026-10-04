// Command bf is the agent-facing Black Fortress CLI: Claude Code hooks, an
// MCP stdio bridge, agent setup and ledger inspection.
package main

import (
	"fmt"
	"os"

	"blackfortress.dev/fortress/bfd/internal/cli"
	"blackfortress.dev/fortress/bfd/internal/runtime"
)

const usage = `bf — Black Fortress agent CLI

Usage:
  bf hook <Event> [--agent NAME]     Claude Code hook handler (reads hook JSON on stdin)
  bf mcp-stdio                       MCP stdio bridge to the local runtime
  bf install-claude [--project DIR]  Install hooks and register the MCP server with Claude Code
  bf agent-config [claude|cursor|codex|stdio]
                                     Print agent configuration
  bf status                          Show runtime status
  bf ledger [verify]                 Show recent evidence, or verify the hash chain
  bf sync                            Upload agent evidence (including today) to Probo now
  bf checks [run [provider]]         Show automated check results, or run them now
  bf version
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	args := os.Args[2:]

	switch os.Args[1] {
	case "hook":
		os.Exit(cli.Hook(args, os.Stdin, os.Stdout))
	case "mcp-stdio":
		os.Exit(cli.MCPStdio(os.Stdin, os.Stdout))
	case "install-claude":
		os.Exit(cli.InstallClaude(args, os.Stdout))
	case "agent-config":
		os.Exit(cli.AgentConfig(args, os.Stdout))
	case "status":
		os.Exit(cli.Status(os.Stdout))
	case "checks":
		os.Exit(cli.Checks(args, os.Stdout))
	case "sync":
		os.Exit(cli.Sync(os.Stdout))
	case "ledger":
		os.Exit(cli.Ledger(args, os.Stdout))
	case "version", "--version":
		fmt.Println(runtime.Version)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
}
