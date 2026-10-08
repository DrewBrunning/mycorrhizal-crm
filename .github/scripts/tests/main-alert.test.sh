#!/usr/bin/env bash
#
# Tests for ../main-alert/ensure-alert.sh (issue #1618).
#
# What these pin: a `cancelled` push run on `main` is only a real failure when
# nothing newer is coming to report the truth. GitHub cancels a concurrency
# group's older *pending* run when a newer one queues, so a burst of merges
# produced four false p1 alerts (#1611/#1614/#1616/#1617). A cancelled run
# with a newer push run of the same workflow must be silent; a lone cancelled
# run still alerts, labelled "cancelled (not superseded)". `failure` is
# unchanged, `success` still closes, and the superseded check fails toward
# alerting when it cannot resolve the ordering.
#
# `gh` is a stub on PATH: it serves scripted JSON per endpoint, applies any
# `--jq` filter the caller passed, and records mutating (`--method POST/PATCH`)
# calls so a scenario can prove "no alert" really means no mutation.

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$SCRIPT_DIR/../main-alert/ensure-alert.sh"

indent() { printf "    | %s\n" "${1//$'\n'/$'\n    | '}"; }

pass=0
fail=0

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/bin"

cat > "$work/bin/gh" <<'STUB'
#!/usr/bin/env bash
printf 'gh %s\n' "$*" >> "$GH_LOG"
jqfilter=""
prev=""
for a in "$@"; do
  if [ "$prev" = "--jq" ] || [ "$prev" = "-q" ]; then jqfilter="$a"; fi
  prev="$a"
done
args="$*"
resp=""
case "$args" in
  *"method POST"* | *"method PATCH"*)
    printf 'MUTATE: %s\n' "$args" >> "$GH_LOG"
    resp='{}'
    ;;
  *"label create"*) exit 0 ;;
  *"actions/runs/${RUN_ID}/jobs"*) resp="$(cat "$JOBS_JSON")" ;;
  *"actions/workflows/"*"/runs?"*) resp="$(cat "$WORKFLOW_RUNS_JSON")" ;;
  *"actions/runs/${RUN_ID}"*) resp="$(cat "$RUN_JSON")" ;;
  *"commits/"*"/pulls"*) resp="$(cat "$PULLS_JSON")" ;;
  *"search/issues"*) resp="$(cat "$SEARCH_JSON")" ;;
  *) resp='{}' ;;
esac
if [ -n "$jqfilter" ]; then
  printf '%s' "$resp" | jq -r "$jqfilter"
else
  printf '%s\n' "$resp"
fi
STUB
chmod +x "$work/bin/gh"

# Scripted API responses; scenarios rewrite the files they care about.
seed() {
  printf '{"workflow_id":123,"run_number":10}\n' > "$work/run.json"
  printf '{"workflow_runs":[{"run_number":10}]}\n' > "$work/workflow-runs.json"
  printf '{"jobs":[{"name":"backend-tests","conclusion":"failure"}]}\n' > "$work/jobs.json"
  printf '[{"number":1599}]\n' > "$work/pulls.json"
  printf '{"items":[]}\n' > "$work/search.json"
}
seed

# scenario NAME WANT_RC WANT_SUBSTRING WANT_ABSENT MUTATION -- ENV=VAL...
# MUTATION is "mutate" (a POST/PATCH must have happened), "nomutate" (it must
# not have), or "" (don't care).
scenario() {
  local name="$1" want_rc="$2" want="$3" absent="$4" mutation="$5"
  shift 5
  [ "${1:-}" = "--" ] && shift
  : > "$work/gh.log"
  local out rc
  out="$(env PATH="$work/bin:$PATH" \
    GH_LOG="$work/gh.log" GH_REPO="Owner/Repo" GH_TOKEN="x" \
    WORKFLOW_NAME="Unit Tests" RUN_ID="1000" RUN_URL="https://example.test/run/1000" \
    RUN_ATTEMPT="1" HEAD_SHA="deadbeef" HEAD_BRANCH="main" \
    RUN_JSON="$work/run.json" WORKFLOW_RUNS_JSON="$work/workflow-runs.json" \
    JOBS_JSON="$work/jobs.json" PULLS_JSON="$work/pulls.json" SEARCH_JSON="$work/search.json" \
    "$@" bash "$SCRIPT" 2>&1)"
  rc=$?

  local ok=1 reason=""
  if [ "$rc" -ne "$want_rc" ]; then ok=0; reason="rc=$rc want=$want_rc"; fi
  if [ "$ok" -eq 1 ] && [ -n "$want" ] && ! grep -qF -- "$want" <<<"$out"; then
    ok=0; reason="missing substring '$want'"
  fi
  if [ "$ok" -eq 1 ] && [ -n "$absent" ] && grep -qF -- "$absent" <<<"$out"; then
    ok=0; reason="unexpected substring '$absent'"
  fi
  local mutated=1
  grep -q '^MUTATE' "$work/gh.log" || mutated=0
  if [ "$ok" -eq 1 ] && [ "$mutation" = "mutate" ] && [ "$mutated" -eq 0 ]; then
    ok=0; reason="expected a mutation, none happened"
  fi
  if [ "$ok" -eq 1 ] && [ "$mutation" = "nomutate" ] && [ "$mutated" -eq 1 ]; then
    ok=0; reason="expected no mutation, one happened"
  fi

  if [ "$ok" -eq 1 ]; then
    pass=$((pass + 1))
    echo "PASS: $name"
  else
    fail=$((fail + 1))
    echo "FAIL: $name ($reason)"
    indent "$out"
  fi
}

# --- cancelled: superseded -------------------------------------------------
# GitHub cancels the older pending push run when a newer one queues.
printf '{"workflow_runs":[{"run_number":11}]}\n' > "$work/workflow-runs.json"
scenario "superseded cancelled is silent (no alert, no mutation)" 0 \
  "superseded by a newer push run" "would open a new issue" nomutate -- \
  CONCLUSION=cancelled DRY_RUN=1

seed
printf '{"workflow_runs":[{"run_number":11}]}\n' > "$work/workflow-runs.json"
scenario "superseded cancelled is silent even when live" 0 \
  "superseded by a newer push run" "Opened a new main-failure issue" nomutate -- \
  CONCLUSION=cancelled DRY_RUN=0

# --- cancelled: lone -------------------------------------------------------
seed
scenario "lone cancelled still alerts" 0 \
  "cancelled (not superseded)" "superseded by a newer push run" nomutate -- \
  CONCLUSION=cancelled DRY_RUN=1

seed
scenario "lone cancelled live opens an issue" 0 \
  "Opened a new main-failure issue" "superseded by a newer push run" mutate -- \
  CONCLUSION=cancelled DRY_RUN=0

seed
scenario "cancelled override NEWER_RUN_EXISTS=1 is superseded" 0 \
  "superseded by a newer push run" "would open a new issue" nomutate -- \
  CONCLUSION=cancelled DRY_RUN=1 NEWER_RUN_EXISTS=1

seed
scenario "cancelled override NEWER_RUN_EXISTS=0 is a real failure" 0 \
  "cancelled (not superseded)" "superseded by a newer push run" nomutate -- \
  CONCLUSION=cancelled DRY_RUN=1 NEWER_RUN_EXISTS=0

# A run whose ordering cannot be resolved must alert rather than go silent.
printf '{}\n' > "$work/run.json"
scenario "cancelled with an unresolvable run alerts (fails open)" 0 \
  "treating the cancellation as a real failure" "superseded by a newer push run" nomutate -- \
  CONCLUSION=cancelled DRY_RUN=1

# --- failure unchanged -----------------------------------------------------
seed
scenario "failure alerts as before" 0 \
  "ended \`failure\`" "superseded" nomutate -- \
  CONCLUSION=failure DRY_RUN=1

seed
scenario "failure live opens an issue" 0 \
  "Opened a new main-failure issue" "" mutate -- \
  CONCLUSION=failure DRY_RUN=0

# --- success unchanged -----------------------------------------------------
seed
printf '{"items":[{"number":42}]}\n' > "$work/search.json"
scenario "success closes the open issue" 0 \
  "would comment on and close issue #42" "" nomutate -- \
  CONCLUSION=success DRY_RUN=1

seed
scenario "success with no open issue does nothing" 0 \
  "nothing to close" "" nomutate -- \
  CONCLUSION=success DRY_RUN=1

echo
echo "main-alert tests: ${pass} passed, ${fail} failed"
[ "$fail" -eq 0 ]
