#!/usr/bin/env bash
# End-to-end smoke test of the Black Fortress runtime on a fresh data
# directory: provisioning, framework import, MCP proxy, console login,
# guardrail hooks, ledger integrity, evidence sync and the checks API.
#
# Usage: fortress/scripts/smoke-test.sh BIN_DIR
#   BIN_DIR holds bfd, bf, probod, probod-bootstrap (and optionally
#   bf-checks) plus library/ — e.g. fortress/bfd/dist/linux-amd64.
# Must run as a non-root user (PostgreSQL refuses root).
set -euo pipefail

BIN_DIR="$(cd "${1:?usage: smoke-test.sh BIN_DIR}" && pwd)"
export BF_HOME="${BF_HOME:-$(mktemp -d)}"
mkdir -p "$BF_HOME"
export BF_BIN_DIR="$BIN_DIR"
export NO_PROXY="localhost,127.0.0.1${NO_PROXY:+,$NO_PROXY}"
CONTROL="http://localhost:${BF_CONTROL_PORT:-7811}"
BF="$BIN_DIR/bf"
LOG="$BF_HOME/smoke-bfd.log"
failures=0

pass() { printf '  \033[32m✓\033[0m %s\n' "$1"; }
fail() { printf '  \033[31m✗\033[0m %s\n' "$1"; failures=$((failures + 1)); }
expect() { if eval "$2"; then pass "$1"; else fail "$1"; fi; }
json() { python3 -c "import json,sys; d=json.load(sys.stdin); print($1)"; }
token() { python3 -c "import json; print(json.load(open('$BF_HOME/secrets.json'))['mcp_token'])"; }

cleanup() {
  if [[ -n "${BFD_PID:-}" ]] && kill -0 "$BFD_PID" 2>/dev/null; then
    kill -INT "$BFD_PID"
    wait "$BFD_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

echo "Black Fortress smoke test (data in $BF_HOME)"
"$BIN_DIR/bfd" run >"$LOG" 2>&1 &
BFD_PID=$!

state=""
for _ in $(seq 1 120); do
  state="$(curl -s "$CONTROL/v1/status" | json 'd["state"]' 2>/dev/null || true)"
  [[ "$state" == running || "$state" == error ]] && break
  sleep 1
done

if [[ "$state" != running ]]; then
  echo "runtime did not start (state: ${state:-none})"
  tail -20 "$LOG"
  tail -20 "$BF_HOME/logs/probod.log" 2>/dev/null || true
  exit 1
fi
pass "runtime running"

status="$(curl -s "$CONTROL/v1/status")"
expect "organization provisioned" '[[ -n "$(json "d.get(\"organization_id\",\"\")" <<<"$status")" ]]'
expect "all services up" '[[ "$(json "all(v in (\"up\",\"disabled\") for v in d[\"services\"].values())" <<<"$status")" == True ]]'

TOKEN="$(token)"
AUTH=(-H "Authorization: Bearer $TOKEN")

frameworks="$(curl -s "${AUTH[@]}" "$CONTROL/v1/posture" | json 'len(d["frameworks"])')"
expect "default frameworks imported ($frameworks)" '[[ "$frameworks" -ge 3 ]]'

expect "API requires the local token" '[[ "$(curl -s -o /dev/null -w "%{http_code}" "$CONTROL/v1/posture")" == 401 && "$(curl -s -o /dev/null -w "%{http_code}" "$CONTROL/v1/login-link")" == 401 ]]'
expect "DNS-rebinding Host rejected" '[[ "$(curl -s -o /dev/null -w "%{http_code}" -H "Host: attacker.example:${BF_CONTROL_PORT:-7811}" "$CONTROL/v1/status")" == 403 ]]'
STORAGE="http://127.0.0.1:${BF_STORAGE_PORT:-7812}"
expect "object store rejects unsigned writes" '[[ "$(curl -s -o /dev/null -w "%{http_code}" -X PUT --data x "$STORAGE/probod/smoke")" == 403 ]]'
expect "object store rejects foreign origins" '[[ "$(curl -s -o /dev/null -w "%{http_code}" -H "Origin: https://evil.example" "$STORAGE/probod/")" == 403 ]]'

policies="$(python3 -c "import json; print(len(json.load(open('$BF_HOME/sync.json')).get('documents',{})))" 2>/dev/null || echo 0)"
expect "Comp policy templates imported as documents ($policies)" '[[ "$policies" -gt 10 ]]'

MCP=(-s -X POST "$CONTROL/mcp" -H "Content-Type: application/json" -H "Accept: application/json, text/event-stream")
code="$(curl -o /dev/null -w '%{http_code}' "${MCP[@]}" -d '{}')"
expect "MCP rejects missing token" '[[ "$code" == 401 ]]'

tools="$(curl "${MCP[@]}" -H "Authorization: Bearer $TOKEN" -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' \
  | sed -n 's/^data: //p' | json 'len(d["result"]["tools"])')"
expect "MCP lists Probo tools ($tools)" '[[ "$tools" -gt 300 ]]'

ORG="$(json 'd["organization_id"]' <<<"$status")"
listed="$(curl "${MCP[@]}" -H "Authorization: Bearer $TOKEN" -H "User-Agent: smoke-test" \
  -d "{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/call\",\"params\":{\"name\":\"listFrameworks\",\"arguments\":{\"organization_id\":\"$ORG\"}}}" \
  | sed -n 's/^data: //p' | json 'len(d["result"]["structuredContent"]["frameworks"])')"
expect "MCP tool call works ($listed frameworks)" '[[ "$listed" == "$frameworks" ]]'

link="$(curl -s "${AUTH[@]}" "$CONTROL/v1/login-link" | json 'd["url"]')"
redirect="$(curl -s -o /dev/null -w '%{redirect_url}' "$link")"
expect "console login redirects to organization" '[[ "$redirect" == */organizations/* ]]'
expect "login link is single-use" '[[ "$(curl -s -o /dev/null -w "%{http_code}" "$link")" == 403 ]]'
expect "foreign origins rejected" '[[ "$(curl -s -o /dev/null -w "%{http_code}" -H "Origin: https://evil.example" "$CONTROL/v1/status")" == 403 ]]'

hook() { "$BF" hook <<<"$1"; }
deny="$(hook '{"session_id":"smoke","cwd":"/tmp","hook_event_name":"PreToolUse","tool_name":"Write","tool_input":{"file_path":"a.ts","content":"k=\"AKIAABCDEFGHIJKLMNOP\""}}')"
expect "hook blocks hard-coded credentials" '[[ "$deny" == *"\"deny\""* ]]'
ask="$(hook '{"session_id":"smoke","cwd":"/tmp","hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"git push origin main --force"}}')"
expect "hook asks before force push" '[[ "$ask" == *"\"ask\""* ]]'
own="$(hook "{\"session_id\":\"smoke\",\"cwd\":\"/tmp\",\"hook_event_name\":\"PreToolUse\",\"tool_name\":\"Read\",\"tool_input\":{\"file_path\":\"$BF_HOME/secrets.json\"}}")"
expect "hook blocks agents reading Black Fortress secrets" '[[ "$own" == *"\"deny\""* ]]'
quiet="$(hook '{"session_id":"smoke","cwd":"/tmp","hook_event_name":"PreToolUse","tool_name":"Edit","tool_input":{"file_path":"infra/main.tf","new_string":"x"}}')"
expect "hook records IaC change silently" '[[ -z "$quiet" ]]'
expect "ledger hash chain verifies" '"$BF" ledger verify >/dev/null'

sync="$("$BF" sync)"
expect "agent evidence uploads to Probo" '[[ "$sync" == *"uploaded"* && "$sync" != *"failed"* ]]'

sleep 3
in_progress="$(curl -s "${AUTH[@]}" "$CONTROL/v1/posture" | json 'sum(f["measures"].get("IN_PROGRESS",0) for f in d["frameworks"])')"
expect "guardrail measures mapped to controls ($in_progress)" '[[ "$in_progress" -gt 0 ]]'
expect "checks API responds" '[[ "$(curl -s -o /dev/null -w "%{http_code}" "${AUTH[@]}" "$CONTROL/v1/checks")" == 200 ]]'

kill -INT "$BFD_PID"
wait "$BFD_PID" 2>/dev/null || true
BFD_PID=""
expect "clean shutdown stops PostgreSQL" '! pgrep -f "postgres -D $BF_HOME" >/dev/null'

echo
if [[ $failures -gt 0 ]]; then
  echo "$failures check(s) failed; bfd log: $LOG"
  exit 1
fi
echo "all smoke checks passed"
