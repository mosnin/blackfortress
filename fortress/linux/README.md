# Black Fortress on Linux

The runtime runs headless on any x86-64 glibc Linux machine: a developer
box, a cloud VM or sandbox where coding agents work, a container, or a CI
runner. Everything ships in the release tarball, including PostgreSQL, and
listens on loopback only.

## Install

```sh
tar -xzf blackfortress-linux-amd64.tar.gz
blackfortress-linux-amd64/install.sh   # root: /opt/blackfortress + /usr/local/bin
                                        # user: ~/.local/opt/blackfortress + ~/.local/bin
bf up                                   # starts in the background, waits until ready (~30 s first time)
```

Releases are published from tags named `fortress-v*`. To fetch the latest
one: `gh release download --repo mosnin/blackfortress --pattern 'blackfortress-linux-amd64.tar.gz*'`.

Running as root is supported (cloud sandboxes and containers usually are):
PostgreSQL refuses root, so bfd runs it as a system user `blackfortress`
(created on first start; `BF_PG_USER` to choose another), and the data
directory defaults to `/var/lib/blackfortress`. Install under `/opt` so that
user can read the binaries.

## Put an agent under it

```sh
bf install-claude                 # Claude Code: guardrail hooks + MCP server
bf agent-config cursor            # or codex, stdio, http
```

From then on every tool call the agent makes is checked against the
guardrails (credentials in files are blocked, force pushes and `curl | sh`
need approval, infrastructure and dependency changes are recorded), written
to the tamper-evident ledger, and mapped to SOC 2, ISO 27001, GDPR, HIPAA,
PCI DSS and CCPA controls. Through MCP the agent can read and update the
organization's frameworks, controls, measures, risks, policies and tasks.
Agents cannot stop the runtime, read its secrets or remove their hooks.

## Get the report back

```sh
bf report                          # Markdown: verdict, activity, guardrail hits, check findings, posture, ledger integrity
bf report --format json --out report.json
bf report --session <id>           # one agent session
bf report --since 2h               # or an RFC 3339 time
bf report --fail-on fail           # exit 1 on FAIL (or --fail-on attention), to gate CI
```

The verdict is **FAIL** when the evidence ledger does not verify or an
automated check has findings, **NEEDS ATTENTION** when guardrails blocked
or asked about something, a check could not run or the runtime is down, and
**PASS** otherwise.

Set `BF_REPORT_DIR` in the agent's environment and each Claude Code session
writes `blackfortress-<session>.md` and `.json` there when it ends, so the
operator can collect them (upload as a CI artifact, post as a pull request
comment, copy to a bucket) without asking the agent.

## A cloud agent sandbox, end to end

Setup script (runs once when the sandbox starts):

```sh
gh release download --repo mosnin/blackfortress --pattern 'blackfortress-linux-amd64.tar.gz' --dir /tmp
tar -xzf /tmp/blackfortress-linux-amd64.tar.gz -C /tmp
/tmp/blackfortress-linux-amd64/install.sh
bf up
bf install-claude
export BF_REPORT_DIR=/workspace/compliance-reports
```

The agent then works normally. At the end, `bf report` (or the files in
`BF_REPORT_DIR`) is what comes back to you. Automated checks run 30 seconds
after start and every 6 hours with whatever credentials the machine has
(`gh`, `aws`, `gcloud`, `az`, or `BF_*_TOKEN` variables); `bf checks run`
runs them now.

## Container

```sh
docker build -t blackfortress -f fortress/linux/Dockerfile blackfortress-linux-amd64/
docker run --rm -it blackfortress            # bfd run in the foreground
```

Use it as a base image for agent containers, or run `bf up` inside an
existing one after `install.sh`.

## Commands

| | |
|---|---|
| `bf up` / `bf down` | start in the background and wait / stop and wait |
| `bf status` | runtime status, console and MCP URLs |
| `bf checks [run [provider]]` | automated check results, or run them now |
| `bf ledger [verify]` | recent evidence, or verify the hash chain |
| `bf sync` | upload evidence to the GRC record now |
| `bf report` | the compliance report above |

Logs are in `$BF_HOME/logs/`. Ports 7810–7814 on 127.0.0.1 (`BF_*_PORT` to
change). The console is at http://localhost:7810. On a remote machine,
forward both ports (`ssh -L 7810:localhost:7810 -L 7811:localhost:7811 host`),
run `bf login-link` there and open the one-time link it prints locally.
