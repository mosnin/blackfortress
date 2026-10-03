# Black Fortress — local runtime architecture

Black Fortress is a developer-machine compliance platform. It runs the Probo
GRC engine (`../probo`) locally, extends it with Comp's control library and
checks (`../`), and exposes it to coding agents (Claude Code, Cursor, Codex)
over MCP and agent hooks. A native macOS app is the UI shell.

```
fortress/
  ARCHITECTURE.md   this file — the contract between components
  bfd/              Go module: runtime daemon + `bf` CLI (hooks, status, config)
  library/          unified control library (Comp + Probo) and the converter
  macos/            SwiftUI app (menu bar + window), bundles bfd and probod
```

## Processes

`bfd run` is the single entrypoint. It supervises:

| Component | Listens on | Notes |
|---|---|---|
| PostgreSQL | unix socket in `$BF_HOME/pg/run`, TCP `127.0.0.1:7814` | bundled binaries, `initdb` on first run |
| Object storage | `127.0.0.1:7812` | in-process S3-compatible server, files under `$BF_HOME/objects` |
| Mail sink | `127.0.0.1:7813` | in-process SMTP, messages saved to `$BF_HOME/mail/*.eml` |
| probod | `127.0.0.1:7810` | Probo server (console, MCP, GraphQL) |
| Chrome | optional | used for PDF export if a local Chrome/Chromium is found |
| bfd control API | `127.0.0.1:7811` | status, events, hook ingestion, auto-login |

All listeners bind to loopback only. Nothing is reachable from the network.

## Data directory (`$BF_HOME`)

Default `~/Library/Application Support/BlackFortress` on macOS,
`$XDG_DATA_HOME/blackfortress` (or `~/.local/share/blackfortress`) elsewhere.
Override with `BF_HOME`.

```
$BF_HOME/
  secrets.json        0600 — generated encryption key, cookie secret, pepper,
                      local user password, personal API key
  probod.yml          generated probod config
  pg/                 postgres data + run dir
  objects/            object storage
  mail/               captured outbound mail
  ledger/             append-only agent evidence ledger (JSONL per day)
  policy.json         guardrail policy (editable)
  logs/               component logs
```

## First-run provisioning

On first start bfd creates a local identity (`owner@blackfortress.local`,
random password), an organization named after the machine user, and a
personal API key, using Probo's connect/console GraphQL APIs. Credentials are
stored in `secrets.json`. The API key is what agents use for MCP.

## bfd control API (`http://127.0.0.1:7811`)

| Method | Path | Purpose |
|---|---|---|
| GET | `/v1/status` | `{"state":"starting|running|degraded|error","services":{"postgres":"up",...},"console_url":...,"mcp_url":...,"organization_id":...,"version":...}` |
| GET | `/v1/events` | Server-Sent Events. Event types: `status`, `guardrail` (a blocked or flagged agent action), `evidence` (ledger entry recorded), `posture` (compliance summary changed) |
| GET | `/v1/posture` | Compliance summary: per framework counts of controls/measures by state, open tasks, recent guardrail hits |
| GET | `/v1/ledger?limit=N` | Recent evidence ledger entries |
| POST | `/v1/hooks/{event}` | Ingest an agent hook payload (used by `bf hook`) and return a decision |
| GET | `/login` | Signs the local user in to probod and redirects to the console (sets the session cookie for `localhost`) |

## `bf` CLI

| Command | Purpose |
|---|---|
| `bf hook <event>` | Claude Code hook handler. Reads the hook JSON on stdin, evaluates guardrails locally, forwards to bfd for the ledger, writes the hook decision JSON to stdout |
| `bf status` | Prints `/v1/status` |
| `bf agent-config [claude|cursor|codex]` | Prints MCP + hook configuration for an agent |
| `bf install-claude [--project DIR]` | Writes the MCP server and hooks into Claude Code settings |

## Agent guardrails and evidence

Guardrails map agent actions to controls. Each rule has an id, a matcher
(tool name + path/command/content patterns), an action (`block`, `ask`,
`record`), and control references (e.g. `SOC2:CC8.1`, `ISO27001:A.8.32`).
Every evaluated action is written to the ledger with the session id, tool,
target, decision, and control references, so change-management and
secure-development evidence accumulates as a side effect of coding.

## Branding

Name: **Black Fortress**. Palette: black background (`#000000`, surfaces
`#0A0A0A`/`#141414`), lime green accent (`#A3E635`, hover `#BEF264`,
pressed `#84CC16`), white text (`#FFFFFF`, secondary `#A3A3A3`).
