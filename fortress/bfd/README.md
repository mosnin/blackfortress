# bfd — Black Fortress runtime

`bfd` runs the whole Black Fortress stack on a developer machine; `bf` connects
coding agents to it. See [`../ARCHITECTURE.md`](../ARCHITECTURE.md) for the
contract between components.

## What `bfd run` does

1. Generates machine-local secrets in `$BF_HOME/secrets.json` (0600).
2. Starts an S3-compatible object store (files under `$BF_HOME/objects`) and
   an SMTP sink (mail saved to `$BF_HOME/mail`), both in-process.
3. Initializes and starts PostgreSQL (`$BF_HOME/pg`).
4. Generates probod's config with `probod-bootstrap` and starts probod on
   `localhost:7810`. Headless Chrome is started for PDF export when one is
   installed.
5. First run only: creates the local owner, verifies its email from the
   mail sink, creates the organization, mints a 90-day agent token (renewed
   automatically), closes sign-up, and imports SOC 2, ISO 27001:2022 and
   GDPR (`BF_DEFAULT_FRAMEWORKS` to change), then creates draft policy
   documents from Comp's policy templates for those frameworks, linked to
   their controls (`BF_IMPORT_POLICIES=0` to skip). Put company details in
   `$BF_HOME/company.json` (`INDUSTRY`, `EMPLOYEES`, `DEVICES`, `SOFTWARE`,
   `LOCATION`, `CRITICAL`, `DATA`, `GEO`, `COMPANYINFO`) before first start
   to fill policy placeholders; unset ones read "[to be defined: …]".
6. Serves the control API, MCP proxy and console login on `localhost:7811`.

Everything listens on loopback only. First start takes a few seconds.

## Connecting agents

```sh
bf install-claude            # hooks in ~/.claude/settings.json + `claude mcp add`
bf agent-config cursor       # ~/.cursor/mcp.json snippet
bf agent-config codex        # stdio bridge for ~/.codex/config.toml
```

Agents talk to `http://localhost:7811/mcp` with a local bearer token; bfd
forwards to Probo's MCP server (378 tools) with the provisioned OAuth token and
records every tool call in the ledger. Agents never see Probo credentials.

## Guardrails and evidence

`bf hook <Event>` is the Claude Code hook handler. Each tool call is evaluated
against [`internal/guard/default_policy.json`](internal/guard/default_policy.json):

| Action | Effect |
|---|---|
| `block` | the tool call is denied (e.g. credentials written into a file) |
| `ask` | the user must approve (e.g. force push, `curl \| sh`, reading `.env`) |
| `record` | allowed, recorded as change-management evidence (IaC, dependencies, auth code, PII schema) |

Every rule carries control references (SOC 2, ISO 27001, GDPR, HIPAA, PCI DSS,
CCPA). Evaluations are written to `$BF_HOME/ledger/YYYY-MM-DD.jsonl`, a
hash-chained log: `bf ledger verify` detects any edited or deleted entry.
Rules are evaluated locally in the hook, so enforcement works even when bfd
is stopped.

Evidence then flows into Probo: bfd maintains three measures (change
management, secret and access safeguards, privacy review) mapped to the
controls their rules cite, and uploads a daily Markdown report of the
matching ledger entries to each. `bf sync` uploads immediately, including
a snapshot of today.

## Automated checks

`bf-checks` (from `../checks`) runs Comp's 49 GitHub, AWS, GCP, Azure,
Vercel, Google Workspace and Aikido checks with the credentials already on
the machine (`gh`, `aws`, `gcloud`, `az` or environment variables). Each
check keeps a Probo measure mapped to the controls its Comp task template
covers; its state follows the result and reports are attached as evidence.
`bf checks` shows the latest results, `bf checks run [provider]` runs them
now, and `$BF_HOME/checks.json` sets the interval, disables providers, or
sets variables such as GitHub `target_repos`.

Agents are also blocked, by rules no policy can override, from reading
`secrets.json`, modifying `$BF_HOME`, or editing Claude Code settings; and
ledger entries are HMAC-signed with a key from `secrets.json`, so they cannot
be rewritten without it.

Customize in `$BF_HOME/policy.json`: a rule with an existing id replaces the
default, `"disabled": true` turns one off, new ids add rules.

## Configuration

| Variable | Default |
|---|---|
| `BF_HOME` | `~/Library/Application Support/BlackFortress` (macOS), `~/.local/share/blackfortress` |
| `BF_BIN_DIR` | directory with `probod` and `probod-bootstrap` (else next to `bfd`, else `PATH`) |
| `BF_PG_DIR` | PostgreSQL install (else Homebrew, Postgres.app, `/usr/lib/postgresql/*`) |
| `BF_GITHUB_TOKEN`, `BF_GCP_TOKEN`, `BF_AZURE_TOKEN`, `BF_VERCEL_TOKEN`, `BF_GOOGLE_WORKSPACE_TOKEN`, `BF_AIKIDO_TOKEN` | override the CLI-discovered credentials for automated checks |
| `BF_LIBRARY_DIR` | framework JSON directory (else `library/frameworks` next to or above `bfd`) |
| `BF_OPENAI_API_KEY`, `BF_ANTHROPIC_API_KEY`, `BF_FIRECRAWL_API_KEY` | enable Probo's AI features (vendor vetting, evidence description) |
| `BF_*_PORT` | `PROBOD` 7810, `CONTROL` 7811, `STORAGE` 7812, `MAIL` 7813, `PG` 7814 |
| `BF_ENABLE_PDF=1` | start headless Chrome for PDF export (off by default: its DevTools port has no authentication) |

Logs: `$BF_HOME/logs/{bfd,probod,postgres}.log`. PostgreSQL refuses to run as
root, so run bfd as a normal user.

## Building

```sh
go test ./...
../scripts/build-runtime.sh darwin-arm64 darwin-amd64   # → dist/<os>-<arch>/
```
