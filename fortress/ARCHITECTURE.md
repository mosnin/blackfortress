# Black Fortress — local runtime architecture

Black Fortress is a developer-machine compliance platform. It runs the Probo
GRC engine (`../probo`) locally, extends it with Comp's control library and
checks (`../`), and exposes it to coding agents (Claude Code, Cursor, Codex)
over MCP and agent hooks. A native macOS app is the UI shell.

```
fortress/
  ARCHITECTURE.md   this file — the contract between components
  bfd/              Go module: runtime daemon + `bf` CLI (hooks, status, config)
  checks/           bf-checks: Comp's integration checks compiled into one Bun binary
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
| GET | `/v1/ledger/verify` | `{"valid":bool,"entries":n,"error"?}` — hash-chain check |
| POST | `/v1/hooks/{event}` | Ingest a ledger entry already written by `bf hook` (bearer token) |
| POST | `/v1/evidence/sync` | Upload agent evidence to Probo now, including today (bearer token) |
| GET | `/v1/checks` | Latest automated check summary per provider (`?detail=1` for every result) |
| POST | `/v1/checks/run` | Run automated checks now, `?provider=github` for one (bearer token) |
| GET | `/v1/login-link` | `{"url": ".../login?nonce=..."}` — one-time, 60 s console login link |
| ANY | `/mcp` | MCP proxy to probod (bearer `mcp_token` from `secrets.json`) |
| GET | `/login?nonce=` | Signs the local user in to probod and redirects to the console (sets the session cookie for `localhost`) |

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

Evidence reaches the GRC record through three Probo measures that bfd
creates and maintains: *AI coding agent change management*, *secret and
access safeguards*, and *privacy review*. Each is mapped to every control
its rules reference in the organization's frameworks, set to In Progress
(marking it Implemented is left to the owner), and receives one Markdown
report per day with the entries, outcome counts and chain-head hash.
Completed days upload at startup and hourly; `bf sync` uploads now.

## Automated checks

`bf-checks` runs Comp's integration checks (GitHub, AWS, GCP, Azure,
Vercel, Google Workspace, Aikido — 49 checks) unchanged, reading one JSON
request on stdin and writing results on stdout. bfd supplies credentials
the developer already has: `gh auth token`, `aws configure
export-credentials`, `gcloud auth print-access-token`, `az account
get-access-token`, or `BF_*` / provider environment variables. GitHub
repositories default to those agents have worked in (from the ledger).

Checks run 30 s after startup and every `interval_minutes` (default 360)
from `$BF_HOME/checks.json`, which also disables providers and sets
variables. Each check becomes a Probo measure *Automated check: <provider>
— <check>*, mapped through its Comp task template to the controls that
template satisfies (via `library/processes/tasks.json`). The measure is
Implemented only with passing evidence and no findings, Not Implemented
with findings, and In Progress when a check found nothing to evaluate. A
Markdown report is uploaded when the result changes, at most once a day
otherwise. Every run is also written to the ledger.

## Branding

Name: **Black Fortress**. Palette: black background (`#000000`, surfaces
`#0A0A0A`/`#141414`), lime green accent (`#A3E635`, hover `#BEF264`,
pressed `#84CC16`), white text (`#FFFFFF`, secondary `#A3A3A3`).
