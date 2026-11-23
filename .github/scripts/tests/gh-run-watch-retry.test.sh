#!/usr/bin/env bash
#
# Tests for ../gh-run-watch-retry.sh (issue #2900).
#
# What these pin: a transient Actions API failure (HTTP 429/5xx while polling
# jobs) is retried with backoff and the watch passes, loudly; a watched run
# that genuinely failed is never retried; a repeat API failure still fails the
# step; and a bad delay value is rejected. `gh` is a stub on PATH replaying
# scripted output per call: call 1 prints $FIRST_OUT and exits $FIRST_RC,
# call 2 prints $SECOND_OUT/$SECOND_RC, and later calls print
# $THIRD_OUT/$THIRD_RC, falling back to the second values.

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$SCRIPT_DIR/../gh-run-watch-retry.sh"

indent() { printf "    | %s\n" "${1//$'\n'/$'\n    | '}"; }

pass=0
fail=0

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/bin"

cat > "$work/bin/gh" <<'STUB'
#!/usr/bin/env bash
echo "gh $*" >> "$GH_LOG"
calls="$(wc -l < "$GH_LOG")"
if [ "$calls" -eq 1 ]; then
  printf '%s\n' "${FIRST_OUT:-}"
  exit "${FIRST_RC:-0}"
fi
if [ "$calls" -eq 2 ]; then
  printf '%s\n' "${SECOND_OUT:-}"
  exit "${SECOND_RC:-0}"
fi
printf '%s\n' "${THIRD_OUT:-${SECOND_OUT:-}}"
exit "${THIRD_RC:-${SECOND_RC:-0}}"
STUB
chmod +x "$work/bin/gh"

API_500='failed to get jobs: HTTP 500 (https://api.github.com/repos/o/r/actions/runs/123/jobs?per_page=100)'
API_429='failed to get jobs: HTTP 429 (https://api.github.com/repos/o/r/actions/runs/123/jobs?per_page=100)'
RUN_FAILED='X main · 123
  X build (1m0s)
Run 123 completed with failure'

# scenario NAME WANT_RC WANT_CALLS WANT_SUBSTRING -- ENV=VAL...
scenario() {
	local name="$1" want_rc="$2" want_calls="$3" want_out="$4"
	shift 5
	: > "$work/gh.log"
	local out rc calls
	# No scenario waits the default 20s backoff; the retry below overrides it.
	out="$(cd "$work" && env PATH="$work/bin:$PATH" GH_LOG="$work/gh.log" GH_RUN_WATCH_DELAY_SECONDS=0 "$@" bash "$SCRIPT" 123 2>&1)"
	rc=$?
	calls="$(wc -l < "$work/gh.log")"
	if [ "$rc" -eq "$want_rc" ] && [ "$calls" -eq "$want_calls" ] && grep -qF -- "$want_out" <<<"$out"; then
		pass=$((pass + 1))
		echo "PASS: $name"
	else
		fail=$((fail + 1))
		echo "FAIL: $name (rc=$rc want=$want_rc, gh calls=$calls want=$want_calls, want substring '$want_out')"
		indent "$out"
	fi
}

scenario "a green run is one gh call" 0 1 "" -- FIRST_RC=0 FIRST_OUT="Run 123 completed with success"
scenario "a failed watched run is not retried" 1 1 "" -- FIRST_RC=1 FIRST_OUT="$RUN_FAILED"
scenario "an HTTP 500 is retried and the watch passes" 0 2 "passed on retry" -- FIRST_RC=1 FIRST_OUT="$API_500" SECOND_RC=0 SECOND_OUT="Run 123 completed with success"
scenario "an HTTP 429 is retried" 0 2 "passed on retry" -- FIRST_RC=1 FIRST_OUT="$API_429" SECOND_RC=0 SECOND_OUT="Run 123 completed with success"
scenario "an API failure that repeats still fails" 1 3 "failed again after a transient" -- FIRST_RC=1 FIRST_OUT="$API_500" SECOND_RC=1 SECOND_OUT="$API_500"
scenario "the retry waits the configured backoff" 0 2 "waiting 1s before retrying" -- GH_RUN_WATCH_DELAY_SECONDS=1 FIRST_RC=1 FIRST_OUT="$API_500" SECOND_RC=0
scenario "a non-numeric retry delay is rejected" 2 0 "must be a non-negative integer" -- GH_RUN_WATCH_DELAY_SECONDS=soon FIRST_RC=1 FIRST_OUT="$API_500"
scenario "a non-numeric attempts value is rejected" 2 0 "must be a positive integer" -- GH_RUN_WATCH_MAX_ATTEMPTS=twice FIRST_RC=1 FIRST_OUT="$API_500"

# The run id must reach gh verbatim.
: > "$work/gh.log"
(cd "$work" && PATH="$work/bin:$PATH" GH_LOG="$work/gh.log" FIRST_RC=0 bash "$SCRIPT" 456 >/dev/null 2>&1)
if grep -qxF "gh run watch 456 --exit-status" "$work/gh.log"; then
	pass=$((pass + 1))
	echo "PASS: the run id is passed through verbatim"
else
	fail=$((fail + 1))
	echo "FAIL: the run id was not passed through verbatim"
	indent "$(cat "$work/gh.log")"
fi

echo
echo "passed: $pass, failed: $fail"
[ "$fail" -eq 0 ]
