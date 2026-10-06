#!/usr/bin/env bash
# End-to-end test: run the service against Lunar's real test program and
# check that every message arrived and was applied.
#
# Usage: scripts/e2e.sh [extra rockets launch flags]
#   e.g. scripts/e2e.sh --max-messages=2000   (quick run; default is 100k)
set -euo pipefail

ROCKETS="${ROCKETS:-./rockets}"
BIN="${BIN:-bin/rockets-service}"
PORT="${PORT:-8089}" # not 8088, so it doesn't clash with a dev server from `make run`
BASE="http://localhost:$PORT"

fail() { echo "e2e: FAIL: $*" >&2; exit 1; }

# print_rockets shows the final state as a table, fastest first.
print_rockets() {
	if ! command -v jq >/dev/null; then
		echo "$1" # no jq: raw JSON is still readable
		return
	fi
	{
		printf 'CHANNEL\tTYPE\tMISSION\tSPEED\tEXPLODED\tLAST APPLIED\tPENDING\n'
		jq -r '.[] | [
			.channel[0:8],
			(if .type == "" then "-" else .type end),
			(if .mission == "" then "-" else .mission end),
			.speed,
			(if .exploded then .explosionReason else "-" end),
			.lastAppliedMessage,
			.pendingMessages
		] | @tsv' <<<"$1"
	} | column -t -s $'\t'
}

[[ -x "$ROCKETS" ]] || fail "test program not found at $ROCKETS (set ROCKETS=path/to/rockets)"
[[ -x "$BIN" ]] || fail "service binary not found at $BIN (run: make build)"

logs="$(mktemp -d)"
"$BIN" -addr ":$PORT" >"$logs/service.log" 2>&1 &
service=$!
trap 'kill "$service" 2>/dev/null || true; wait "$service" 2>/dev/null || true' EXIT

for _ in $(seq 50); do
	curl -sf "$BASE/rockets" >/dev/null && break
	sleep 0.1
done
curl -sf "$BASE/rockets" >/dev/null || fail "service didn't start; see $logs/service.log"

echo "e2e: running the test program against $BASE (logs in $logs)"
start=$SECONDS
"$ROCKETS" launch "$BASE/messages" --log-level error "$@" >"$logs/rockets.log" 2>&1
echo "e2e: test program finished in $((SECONDS - start))s"

rockets=$(curl -sf "$BASE/rockets?sort=speed&order=desc")
echo
print_rockets "$rockets"
echo

# The test program exits 0 even when deliveries fail, so check its log.
errors=$(grep -c '\[error\]' "$logs/rockets.log" || true)
[[ "$errors" -eq 0 ]] || fail "test program logged $errors delivery errors; see $logs/rockets.log"

count=$({ grep -o '"channel"' <<<"$rockets" || true; } | wc -l | tr -d ' ')
[[ "$count" -gt 0 ]] || fail "no rockets in the service"

stuck=$({ grep -o '"pendingMessages":[1-9][0-9]*' <<<"$rockets" || true; } | wc -l | tr -d ' ') # grep exits 1 on no match
[[ "$stuck" -eq 0 ]] || fail "$stuck of $count rockets have pending messages: a message never arrived"

echo "e2e: PASS: $count rockets, all messages applied, none pending"
