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
  bf up [--timeout 3m]               Start the runtime in the background and wait until ready
  bf down                            Stop the runtime started by bf up
  bf report [--since 24h] [--session ID] [--format md|json] [--out FILE] [--fail-on fail|attention]
                                     Compliance report: verdict, agent activity, guardrail hits,
                                     check findings, framework posture, ledger integrity
  bf hook <Event> [--agent NAME]     Claude Code hook handler (reads hook JSON on stdin)
  bf mcp-stdio                       MCP stdio bridge to the local runtime
  bf install-claude [--project DIR]  Install hooks and register the MCP server with Claude Code
  bf agent-config [claude|cursor|codex|stdio|http]
                                     Print agent configuration
  bf status                          Show runtime status
  bf login-link                      Print a one-time console sign-in link (60 s)
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
	case "up":
		os.Exit(cli.Up(args, os.Stdout))
	case "down":
		os.Exit(cli.Down(os.Stdout))
	case "report":
		os.Exit(cli.ReportCmd(args, os.Stdout))
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
	case "login-link":
		os.Exit(cli.LoginLink(os.Stdout))
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
