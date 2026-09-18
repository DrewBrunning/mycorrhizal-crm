#!/usr/bin/env bash
#
# Tests for ../release-gate-state.sh (issue #1150).
#
# The bug this pins: "Backend (Go)" is a `needs:`-gated fan-in job, so GitHub
# Actions creates no check-run for it until backend-checks and every
# backend-tests leg finish. On a busy `push: main` those legs can outlast the
# mandatory-gate poll's deadline, and both pollers classified the absent
# check-run as the literal "missing" -- which release-gate-decide.sh turns into
# a hard block at the deadline. But a gate whose owning workflow has a run
# observably in flight on the commit is a timing question (the run just has not
# reached the aggregation job yet), which
# docs/development/release-gates.md already calls not-a-quality-signal. The
# state builder must report that run's status instead of "missing".
#
# It must NOT weaken the #913 guard: a completed workflow run with no
# check-run for the gate (a job filtered out or renamed away) stays "missing".

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$SCRIPT_DIR/../release-gate-state.sh"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

GATES="$TMP/gates.json"
cat > "$GATES" <<'JSON'
{"gates":[
  {"name":"Backend (Go)","workflow":"unit-tests.yml","check_context":"Backend (Go)","release_gate":true},
  {"name":"Frontend (Vitest)","workflow":"unit-tests.yml","check_context":"Frontend (Vitest)","release_gate":true},
  {"name":"Run E2E Tests","workflow":"e2e-tests.yml","check_context":"Run E2E Tests","release_gate":true},
  {"name":"Gone Suite","workflow":"gone.yml","check_context":"Gone Suite","release_gate":true},
  {"name":"Advisory","workflow":"x.yml","check_context":"Advisory","release_gate":false}
]}
JSON

pass=0
fail=0

# run_case <name> <want> <since> <runs> <statuses> <wfruns> <want_state>
run_case() {
  local name="$1" want="$2" since="$3" runs="$4" statuses="$5" wfruns="$6" want_state="$7"

  printf '%s' "$runs" > "$TMP/runs.json"
  printf '%s' "$statuses" > "$TMP/statuses.json"
  printf '%s' "$wfruns" > "$TMP/wfruns.json"

  local got
  if ! got="$("$SCRIPT" "$want" "$since" "$TMP/runs.json" "$TMP/statuses.json" "$TMP/wfruns.json" "$GATES" 2>&1)"; then
    fail=$((fail + 1))
    echo "FAIL: $name (script exited non-zero: $got)"
    return
  fi
  got="$(printf '%s' "$got" | jq -S -c .)"
  local want_norm
  want_norm="$(printf '%s' "$want_state" | jq -S -c .)"

  if [ "$got" = "$want_norm" ]; then
    pass=$((pass + 1))
    echo "PASS: $name"
  else
    fail=$((fail + 1))
    echo "FAIL: $name"
    echo "  want: $want_norm"
    echo "  got:  $got"
  fi
}

# A gate with a completed check-run reports it verbatim.
run_case "a completed check-run is reported verbatim" \
  '["Frontend (Vitest)"]' '{}' \
  '[{"name":"Frontend (Vitest)","status":"completed","conclusion":"success","started_at":"2026-09-18T17:00:00Z"}]' \
  '[]' '[]' \
  '[{"name":"Frontend (Vitest)","status":"completed","conclusion":"success"}]'

# The regression: no check-run, owning workflow in flight -> pending, not missing.
run_case "REGRESSION: a fan-in gate with no check-run but an in-flight workflow is pending, not missing" \
  '["Backend (Go)"]' '{}' \
  '[]' '[]' \
  '[{"path":".github/workflows/unit-tests.yml","status":"in_progress","conclusion":null,"created_at":"2026-09-18T16:56:35Z"}]' \
  '[{"name":"Backend (Go)","status":"in_progress","conclusion":""}]'

run_case "a queued workflow run reports queued" \
  '["Backend (Go)"]' '{}' \
  '[]' '[]' \
  '[{"path":".github/workflows/unit-tests.yml","status":"queued","conclusion":null,"created_at":"2026-09-18T16:56:35Z"}]' \
  '[{"name":"Backend (Go)","status":"queued","conclusion":""}]'

# #913 preserved: a *completed* workflow run with no check-run for the gate is
# the structural absence the hard block exists for.
run_case "REGRESSION GUARD: a completed workflow with no check-run stays missing" \
  '["Backend (Go)"]' '{}' \
  '[]' '[]' \
  '[{"path":".github/workflows/unit-tests.yml","status":"completed","conclusion":"success","created_at":"2026-09-18T16:56:35Z"}]' \
  '[{"name":"Backend (Go)","status":"missing","conclusion":""}]'

run_case "a gate with no check-run and no workflow run stays missing" \
  '["Backend (Go)"]' '{}' \
  '[]' '[]' '[]' \
  '[{"name":"Backend (Go)","status":"missing","conclusion":""}]'

run_case "a failed check-run is reported as completed/failure, not masked by an in-flight workflow" \
  '["Run E2E Tests"]' '{}' \
  '[{"name":"Run E2E Tests","status":"completed","conclusion":"failure","started_at":"2026-09-18T17:00:00Z"}]' \
  '[]' \
  '[{"path":".github/workflows/e2e-tests.yml","status":"in_progress","conclusion":null,"created_at":"2026-09-18T16:56:35Z"}]' \
  '[{"name":"Run E2E Tests","status":"completed","conclusion":"failure"}]'

# #1013 cutoff: a stale in-flight run from a previous dispatch must not keep a
# fresh dispatch looking pending.
run_case "a workflow run older than the dispatch cutoff is ignored" \
  '["Run E2E Tests"]' '{"Run E2E Tests":"2026-09-18T18:00:00Z"}' \
  '[]' '[]' \
  '[{"path":".github/workflows/e2e-tests.yml","status":"in_progress","conclusion":null,"created_at":"2026-09-18T17:00:00Z"}]' \
  '[{"name":"Run E2E Tests","status":"missing","conclusion":""}]'

run_case "a workflow run newer than the dispatch cutoff counts" \
  '["Run E2E Tests"]' '{"Run E2E Tests":"2026-09-18T18:00:00Z"}' \
  '[]' '[]' \
  '[{"path":".github/workflows/e2e-tests.yml","status":"in_progress","conclusion":null,"created_at":"2026-09-18T18:05:00Z"}]' \
  '[{"name":"Run E2E Tests","status":"in_progress","conclusion":""}]'

# The newest of several runs for one workflow wins.
run_case "the newest workflow run for a workflow wins" \
  '["Backend (Go)"]' '{}' \
  '[]' '[]' \
  '[{"path":".github/workflows/unit-tests.yml","status":"completed","conclusion":"success","created_at":"2026-09-18T17:00:00Z"},
    {"path":".github/workflows/unit-tests.yml","status":"in_progress","conclusion":null,"created_at":"2026-09-18T18:00:00Z"}]' \
  '[{"name":"Backend (Go)","status":"in_progress","conclusion":""}]'

# A commit-status (not check-run) gate is still read.
run_case "commit statuses are merged with check-runs" \
  '["Run E2E Tests"]' '{}' \
  '[]' \
  '[{"name":"Run E2E Tests","status":"completed","conclusion":"success"}]' \
  '[]' \
  '[{"name":"Run E2E Tests","status":"completed","conclusion":"success"}]'

echo ""
echo "$pass passed, $fail failed"
if [ "$fail" -ne 0 ]; then
  exit 1
fi
exit 0
