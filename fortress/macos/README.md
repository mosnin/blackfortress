# Black Fortress for macOS

The native shell for Black Fortress: a menu bar extra plus a main window. It
supervises the local runtime (`bfd`) and shows its posture, agent activity,
and the embedded Probo console. The contract with the runtime is
[`../ARCHITECTURE.md`](../ARCHITECTURE.md).

## Requirements

- macOS 14 Sonoma or later
- Xcode 15 or later (Swift 5.9+), or the matching Command Line Tools. The build
  uses SwiftPM only and needs no Xcode project.
- To bundle the runtime: `fortress/scripts/build-runtime.sh`, which produces
  `fortress/bfd/dist/darwin-<arch>/` containing `bfd`, `bf`, `probod`,
  `probod-bootstrap` and `library/` (`library/frameworks/*.json`)
- Network access the first time you build, to download PostgreSQL 16

## Build

```sh
cd fortress/macos

# Optional: fetch PostgreSQL first. build-app.sh does this itself if it is missing.
scripts/fetch-postgres.sh            # host arch -> Resources/postgres/darwin-<arch>
scripts/fetch-postgres.sh all        # arm64 + amd64

# Build the runtime dist first (from the fortress/ directory)
../scripts/build-runtime.sh

# Build build/BlackFortress.app (release build, bundled binaries, ad-hoc signed)
scripts/build-app.sh
open build/BlackFortress.app
```

`build-app.sh` does the following:

1. Runs `swift build -c release`.
2. Assembles `build/BlackFortress.app` and writes an `Info.plist` with:
   - bundle id `dev.blackfortress.app`
   - `LSMinimumSystemVersion` 14.0
   - `LSUIElement` false, so the app has a Dock icon as well as the menu bar extra
   - an ATS exception for `localhost` / `127.0.0.1` (`NSAllowsLocalNetworking`)
3. Copies `bfd`, `bf`, `bf-checks`, `probod` and `probod-bootstrap` from
   `../bfd/dist/darwin-<arch>/` into `Contents/Resources/bin/`, and that
   directory's `library/` into `Contents/Resources/library/`. bfd looks for
   the control library at `<exe dir>/../library/frameworks`.
4. Copies PostgreSQL (`bin/`, `lib/`, `share/`) into `Contents/Resources/postgres/`.
5. Ad-hoc codesigns every nested Mach-O file first, then the app itself.

You can set these environment variables: `VERSION`, `BUILD_NUMBER`, `BFD_DIST`
(another binaries directory), `PG_SRC`, `SKIP_PG=1`, `STRICT=1` (fail when a
runtime binary is missing), `OUT_DIR`, and `SIGN_IDENTITY` (a real Developer ID
instead of `-`).

### PostgreSQL

`scripts/fetch-postgres.sh` downloads the relocatable PostgreSQL build that
[zonky embedded-postgres-binaries](https://github.com/zonkyio/embedded-postgres-binaries)
publishes on Maven Central:

- `io.zonky.test.postgres:embedded-postgres-binaries-darwin-arm64v8`
- `io.zonky.test.postgres:embedded-postgres-binaries-darwin-amd64`

The default version is `16.15.0`; override it with `PG_VERSION=16.x.y`. Each
artifact is a `.jar` that contains a single `postgres-darwin-*.txz`. The script
checks the jar against the `.sha1` that Maven publishes, then unpacks the
archive to `Resources/postgres/darwin-<arch>/`. That directory is git-ignored.

## How the app finds and runs the runtime

At launch, `DaemonSupervisor` takes these steps:

1. It probes `GET http://127.0.0.1:7811/v1/status`. If anything answers, the app
   **attaches** to it. It does not spawn anything, and it never stops that
   process.
2. Otherwise it spawns `bfd run` with this environment:
   - `BF_HOME`: inherited if set, else `~/Library/Application Support/BlackFortress`
   - `BF_BIN_DIR`: the binaries directory (it is also prepended to `PATH`)
   - `BF_PG_DIR`: the PostgreSQL directory, when one is found. bfd runs
     `$BF_PG_DIR/bin/initdb` and `$BF_PG_DIR/bin/postgres`.
3. bfd writes its own logs to `$BF_HOME/logs/{bfd,probod,postgres}.log`. The
   app also appends the child's raw stdout and stderr to
   `$BF_HOME/logs/bfd-macos.log`.
4. If bfd exits unexpectedly, the supervisor restarts it with exponential
   backoff (1, 2, 4 … 30 s). The backoff resets once a run lasts longer than 60 s.
   Before each restart it probes :7811 again and attaches if another bfd took
   over.
5. On quit, the app sends SIGTERM to bfd and waits up to 20 s, then sends SIGKILL.
   The app delays termination until bfd has stopped
   (`applicationShouldTerminate` → `.terminateLater`).

The binaries directory is the first of these that contains an executable `bfd`:

1. `$BF_BIN_DIR`
2. `BlackFortress.app/Contents/Resources/bin`
3. `../bfd/dist/darwin-<arch>` relative to the current directory (for `swift run`)

The PostgreSQL directory is the first of these that exists: `$BF_PG_DIR`, then
`Contents/Resources/postgres`, then `./Resources/postgres`.

If no `bfd` is found, the app stays in attach-only mode. It keeps polling :7811
and attaches as soon as a runtime appears.

## Dev mode: attach to a bfd you run yourself

```sh
# terminal 1: run the daemon by hand (however you build it)
bfd run

# terminal 2: run the app; it sees :7811 answering and attaches
cd fortress/macos && BF_ATTACH_ONLY=1 swift run
```

`BF_ATTACH_ONLY=1` (or `defaults write dev.blackfortress.app attachOnly -bool YES`
for the bundled app) stops the app from ever spawning bfd. Running with
`swift run` works for UI development, with two limits: user notifications are
disabled because the process has no app bundle, and launch at login has no
effect. Use `scripts/build-app.sh` to test those features.

## Features

- **Menu bar extra.** A shield icon tinted by state: lime when everything is
  healthy, white while starting, amber when degraded or restarting, red on
  error. It shows runtime and service status, per-framework posture, the last
  5 guardrail events, and these actions: Open Dashboard, Open Console, Copy MCP
  config for Claude Code, and Quit. The copy action runs
  `bf agent-config claude`. If that fails, it builds an `mcpServers` entry from
  `/v1/status` and the `mcp_token` in `secrets.json`.
  There is no "pause guardrails" action, because the control API documents no
  endpoint for it.
- **Overview.** A status banner, the guardrail counters for the last 24 hours
  (`posture.guardrails`: evaluated, blocked, asked, recorded), posture cards
  per framework (score ring and bar, plus implemented, in-progress and
  not-started controls), service health, and recent agent activity. It updates live
  from the `/v1/events` SSE stream and polls `/v1/status` as a fallback.
- **Agent Activity.** A live table of ledger entries and guardrail events with
  time, session, tool, target, decision and control references. You can filter
  it by text and by decision. Click a row to see its details: hook event,
  outcome, rules, repo, cwd and hash. **Verify ledger integrity** calls
  `GET /v1/ledger/verify` and shows whether the hash chain is intact.
- **Frameworks.** Readiness per framework, with breakdowns by state for
  controls and measures.
- **Console.** A `WKWebView`. Each time the tab opens or reloads, the app
  calls `GET /v1/login-link` and loads the one-time
  `http://localhost:7811/login?nonce=…` URL it returns. The nonce is
  single-use and expires after 60 s. That page signs you in and redirects to
  the console on `localhost:7810`. The app always uses `localhost`, never
  `127.0.0.1`, so the session cookie matches the console's host. Only `localhost`,
  `127.0.0.1` and `::1` may load in the view. Other links open in your default
  browser.
- **Settings.** The data directory (with Reveal in Finder and Show Logs), a
  Restart Runtime button, launch at login (`SMAppService.mainApp`), an
  **Install into Claude Code** button that runs `<bin>/bf install-claude` and
  shows its output, copyable manual commands, and app and runtime versions.
- **Notifications.** A user notification for each `guardrail` event whose
  decision is `block`.

The models use the field names of the bfd Go structs:
`internal/runtime/{daemon,posture,events}.go` and `internal/guard/ledger.go`.
SSE frames are `event: <type>` followed by
`data: {"type","time","data": <payload>}`. The app unwraps `.data` into a
Status, Posture or ledger Entry. Ledger entries are identified by their
`hash`, so the same entry arriving over SSE and from `/v1/ledger` is shown
once. Decoding stays tolerant: missing fields become `nil`, unknown SSE event
types are ignored, and alternate spellings are accepted.

## Troubleshooting

- **Runtime stuck in "Starting" or "Error".** Read the logs in
  `$BF_HOME/logs/`. That is `bfd.log`, `probod.log` and `postgres.log`, plus
  `bfd-macos.log` for what the app captured. **Settings → Show Logs** opens
  them. `/v1/status` reports the failure reason in its `error` field, which
  the Overview banner shows.
- **Do not run bfd as root.** PostgreSQL refuses to start as root. On a Mac
  this only matters if you start bfd by hand with `sudo`. The app always runs
  bfd as the logged-in user.
- **Console shows a sign-in error.** The login nonce is single-use and expires
  after 60 s. Reload the Console tab to get a fresh link.
- **No frameworks.** The bundle is missing `Contents/Resources/library`.
  Rebuild with `fortress/scripts/build-runtime.sh`, then `scripts/build-app.sh`.

## Code layout

```
Package.swift
scripts/build-app.sh          assemble + sign BlackFortress.app
scripts/fetch-postgres.sh     download PostgreSQL 16 (zonky)
Sources/BlackFortress/
  App/      BlackFortressApp (scenes, AppDelegate), AppModel (state store),
            NotificationManager
  Runtime/  DaemonSupervisor, BFDClient, SSEClient (+ SSEParser), Models,
            JSONValue, RuntimePaths, AgentConfig
  Views/    MainWindow (sidebar), Overview, AgentActivity, Frameworks,
            Console (WKWebView), Settings, MenuBarContent, Components
  Theme/    Theme.swift: colors, fonts, button styles, card modifier
```

## Honest note: authored without compiling on macOS

This app was written in a Linux environment that has no macOS SDK, so it has
**not been compiled or run against the real Apple frameworks**. Here is what
was checked:

- Every source file passes `swiftc -parse` (Swift 6.0.3, Swift 5 language mode).
- The Foundation-only runtime layer was compiled and unit-tested on Linux with
  a scratch harness: `JSONValue`, `Models`, `SSEParser`, `BFDClient`,
  `RuntimePaths`, `AgentConfig`, and `DaemonSupervisor` (with a small shim
  standing in for Combine). The tests covered SSE framing (CRLF, comments,
  multi-line data, default event type) and envelope unwrapping. They decode
  the exact status, posture, ledger, ledger-verify and SSE frame shapes that
  bfd emits, including RFC 3339 timestamps with nanoseconds.
- The UI layer was type-checked against hand-written stubs of the SwiftUI,
  AppKit, WebKit, UserNotifications and ServiceManagement APIs it uses. This
  catches internal mistakes (wrong member names, type mismatches, actor
  isolation between the app's own types). It cannot prove that the real Apple
  signatures match the stubs.

The first build on a Mac may still need small fixes. The likeliest spots:
the colored `MenuBarExtra` label image, `NSImage.SymbolConfiguration` palette
colors, and the async forms of the WebKit and UserNotifications delegate
methods.
