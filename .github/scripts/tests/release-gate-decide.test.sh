#!/usr/bin/env bash
#
# Tests for ../release-gate-decide.sh (issue #913, REL-03 finding F3).
#
# The bug this pins: a mandatory `release_gate: true` context that NEVER
# reported a check-run/commit-status on the release commit ("missing") was
# classified identically to one that started but has not concluded yet
# ("pending") -- both just counted as "unfinished", and at the poll's
# deadline BOTH produced only a `::warning::` and let publication proceed.
# Absence is a structural signal (disabled/renamed workflow, a filter
# regression that skips the job entirely, a failed dispatch); a still-running
# check is a timing question. They must not resolve the same way.
#
# Each case feeds a fixed `state` JSON (what the pollers build from the GitHub
# API before handing it to this script) plus a deadline-reached flag, and
# asserts the decision printed on the first line plus the gate names on the
# rest.

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$SCRIPT_DIR/../release-gate-decide.sh"

pass=0
fail=0

run_test() {
  local name="$1" state="$2" deadline_reached="$3" want_decision="$4"
  shift 4
  local want_names=("$@")

  local out status decision names_out ok
  out="$(printf '%s' "$state" | bash "$SCRIPT" "$deadline_reached" 2>&1)"
  status=$?
  decision="$(printf '%s\n' "$out" | head -1)"
  names_out="$(printf '%s\n' "$out" | tail -n +2 | grep -v '^$' || true)"

  ok=1
  [ "$status" -eq 0 ] || ok=0
  [ "$decision" = "$want_decision" ] || ok=0

  for want in "${want_names[@]:-}"; do
    [ -z "$want" ] && continue
    if ! printf '%s\n' "$names_out" | grep -qx "$want"; then
      ok=0
    fi
  done
  # No unexpected extra names.
  local got_count want_count
  got_count="$(printf '%s\n' "$names_out" | grep -c . || true)"
  want_count="${#want_names[@]}"
  [ -z "$names_out" ] && got_count=0
  [ "$got_count" -eq "$want_count" ] || ok=0

  if [ "$ok" -eq 1 ]; then
    pass=$((pass + 1))
    echo "PASS: $name"
  else
    fail=$((fail + 1))
    echo "FAIL: $name (exit=$status decision=$decision want=$want_decision names=[$names_out] want_names=[${want_names[*]:-}])"
  fi
}

state_all_green='[
  {"name":"unit-tests","status":"completed","conclusion":"success"},
  {"name":"e2e-tests","status":"completed","conclusion":"neutral"},
  {"name":"sast","status":"completed","conclusion":"skipped"}
]'
run_test "all gates completed with acceptable conclusions -> PASS" \
  "$state_all_green" false "PASS"

state_one_failed='[
  {"name":"unit-tests","status":"completed","conclusion":"success"},
  {"name":"android-tests","status":"completed","conclusion":"failure"},
  {"name":"sast","status":"queued","conclusion":""}
]'
run_test "an observed failure is a hard block even pre-deadline, other gates still pending" \
  "$state_one_failed" false "FAIL_OBSERVED" "android-tests"

state_one_cancelled='[
  {"name":"unit-tests","status":"completed","conclusion":"success"},
  {"name":"zizmor","status":"completed","conclusion":"cancelled"}
]'
run_test "cancelled conclusion is a hard block" \
  "$state_one_cancelled" true "FAIL_OBSERVED" "zizmor"

state_one_timed_out='[
  {"name":"unit-tests","status":"completed","conclusion":"success"},
  {"name":"e2e-tests","status":"completed","conclusion":"timed_out"}
]'
run_test "timed_out conclusion is a hard block" \
  "$state_one_timed_out" true "FAIL_OBSERVED" "e2e-tests"

state_one_missing='[
  {"name":"unit-tests","status":"completed","conclusion":"success"},
  {"name":"android-tests","status":"missing","conclusion":""}
]'
run_test "a gate that never reported is NOT a failure before the deadline -- keep polling" \
  "$state_one_missing" false "POLL" "android-tests"

state_one_running='[
  {"name":"unit-tests","status":"completed","conclusion":"success"},
  {"name":"e2e-tests","status":"in_progress","conclusion":""}
]'
run_test "a gate that started but is still running -- keep polling pre-deadline" \
  "$state_one_running" false "POLL" "e2e-tests"

run_test "REGRESSION GUARD: a gate that NEVER reported is a hard block at the deadline" \
  "$state_one_missing" true "FAIL_MISSING" "android-tests"

run_test "a gate that started but is still running at the deadline -- warn and proceed" \
  "$state_one_running" true "WARN_PASS" "e2e-tests"

state_missing_and_running='[
  {"name":"unit-tests","status":"completed","conclusion":"success"},
  {"name":"android-tests","status":"missing","conclusion":""},
  {"name":"e2e-tests","status":"in_progress","conclusion":""}
]'
run_test "missing takes priority over a merely-slow gate at the deadline" \
  "$state_missing_and_running" true "FAIL_MISSING" "android-tests"

state_failed_and_missing='[
  {"name":"unit-tests","status":"completed","conclusion":"failure"},
  {"name":"android-tests","status":"missing","conclusion":""}
]'
run_test "an observed failure takes priority over a missing gate" \
  "$state_failed_and_missing" true "FAIL_OBSERVED" "unit-tests"

state_all_missing='[
  {"name":"unit-tests","status":"missing","conclusion":""},
  {"name":"e2e-tests","status":"missing","conclusion":""}
]'
run_test "every gate missing at the deadline lists every gate as FAIL_MISSING" \
  "$state_all_missing" true "FAIL_MISSING" "unit-tests" "e2e-tests"

echo ""
echo "$pass passed, $fail failed"
if [ "$fail" -ne 0 ]; then
  exit 1
fi
exit 0
