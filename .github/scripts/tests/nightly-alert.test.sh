#!/usr/bin/env bash
#
# Tests for ../nightly-alert/ensure-alert.sh and ../nightly-alert/already-reported.sh
# (and the ../alert-lib/issues.sh lookups both share).
#
# What these pin: neither script touches the Search API -- its separate
# 30-requests/minute budget 403'd nightly-failure-reconcile.yml's ~33
# simultaneous matrix legs and then the alert for that failure, so the alarm
# alerted no one (2026-10-13, runs 38400077165 / 38400127969). The REST
# lookup that replaced it matches the issue title exactly (a sibling
# workflow's issue, the main alarm's issue and pull requests never match),
# and the reconcile dedupe finds a run linked from the issue body or any
# comment without mistaking run 12345 for run 123456.
#
# `gh` is a stub on PATH: it serves scripted JSON per endpoint, applies any
# `--jq` filter the caller passed, and records mutating (`--method POST/PATCH`)
# and Search API calls.

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENSURE="$SCRIPT_DIR/../nightly-alert/ensure-alert.sh"
REPORTED="$SCRIPT_DIR/../nightly-alert/already-reported.sh"

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
  *"search/issues"*)
    printf 'SEARCH: %s\n' "$args" >> "$GH_LOG"
    resp='{"items":[],"total_count":0}'
    ;;
  *"actions/runs/${RUN_ID}/jobs"*) resp="$(cat "$JOBS_JSON")" ;;
  *"/issues?labels=nightly-failure&"*) resp="$(cat "$ISSUES_JSON")" ;;
  *"/issues/"*"/comments"*) resp="$(cat "$COMMENTS_JSON")" ;;
  *"/issues/"[0-9]*) resp="$(cat "$ISSUE_JSON")" ;;
  *) resp='{}' ;;
esac
if [ -n "$jqfilter" ]; then
  printf '%s' "$resp" | jq -r "$jqfilter"
else
  printf '%s\n' "$resp"
fi
STUB
chmod +x "$work/bin/gh"

seed() {
  printf '{"jobs":[{"name":"backend-tests","conclusion":"failure"}]}\n' > "$work/jobs.json"
  printf '[]\n' > "$work/issues.json"
  printf '{"body":"nothing here"}\n' > "$work/issue.json"
  printf '[]\n' > "$work/comments.json"
}

# scenario NAME SCRIPT WANT_RC WANT_SUBSTRING WANT_ABSENT MUTATION -- ENV=VAL...
# MUTATION is "mutate", "nomutate", or "" (don't care). The script's
# $GITHUB_OUTPUT is appended to its output so `reported=` can be asserted.
scenario() {
  local name="$1" script="$2" want_rc="$3" want="$4" absent="$5" mutation="$6"
  shift 6
  [ "${1:-}" = "--" ] && shift
  : > "$work/gh.log"
  : > "$work/output"
  local out rc
  out="$(env PATH="$work/bin:$PATH" \
    GH_LOG="$work/gh.log" GH_REPO="Owner/Repo" GH_TOKEN="x" \
    WORKFLOW_NAME="E2E Tests" RUN_ID="12345" RUN_URL="https://github.com/Owner/Repo/actions/runs/12345" \
    RUN_ATTEMPT="1" GITHUB_OUTPUT="$work/output" \
    JOBS_JSON="$work/jobs.json" ISSUES_JSON="$work/issues.json" \
    ISSUE_JSON="$work/issue.json" COMMENTS_JSON="$work/comments.json" \
    "$@" bash "$script" 2>&1)"
  rc=$?
  out+=$'\n'"$(cat "$work/output")"

  local ok=1 reason=""
  if [ "$rc" -ne "$want_rc" ]; then ok=0; reason="rc=$rc want=$want_rc"; fi
  if [ "$ok" -eq 1 ] && [ -n "$want" ] && ! grep -qF -- "$want" <<<"$out"; then
    ok=0; reason="missing substring '$want'"
  fi
  if [ "$ok" -eq 1 ] && [ -n "$absent" ] && grep -qF -- "$absent" <<<"$out"; then
    ok=0; reason="unexpected substring '$absent'"
  fi
  if [ "$ok" -eq 1 ] && grep -q '^SEARCH' "$work/gh.log"; then
    ok=0; reason="called the Search API"
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

# Near-miss titles: a sibling workflow whose name contains this one, the
# red-main alarm's issue for the same workflow, and a pull request.
near_misses='{"number":7,"title":"Nightly failure: Reference-Client E2E Tests"},{"number":8,"title":"Main failure: E2E Tests"},{"number":9,"title":"Nightly failure: E2E Tests","pull_request":{}}'

# --- ensure-alert.sh -------------------------------------------------------
seed
scenario "failure with no issue opens one" "$ENSURE" 0 \
  "Opened a new nightly-failure issue" "" mutate -- CONCLUSION=failure

seed
printf '[%s]\n' "$near_misses" > "$work/issues.json"
scenario "failure ignores near-miss titles and PRs (opens a new issue)" "$ENSURE" 0 \
  "Opened a new nightly-failure issue" "Found existing" mutate -- CONCLUSION=failure

seed
printf '[%s,{"number":42,"title":"Nightly failure: E2E Tests"}]\n' "$near_misses" > "$work/issues.json"
scenario "failure comments on the exact-title issue" "$ENSURE" 0 \
  "Commented on existing issue #42" "" mutate -- CONCLUSION=failure

seed
printf '[{"number":42,"title":"Nightly failure: E2E Tests"}]\n' > "$work/issues.json"
scenario "success closes the open issue" "$ENSURE" 0 \
  "Closed issue #42" "" mutate -- CONCLUSION=success

seed
printf '[%s]\n' "$near_misses" > "$work/issues.json"
scenario "success never closes a near-miss issue" "$ENSURE" 0 \
  "nothing to close" "Closed issue" nomutate -- CONCLUSION=success

# --- already-reported.sh ---------------------------------------------------
seed
scenario "no issue for the workflow -> not reported" "$REPORTED" 0 \
  "reported=false" "reported=true" nomutate

seed
printf '[{"number":42,"title":"Nightly failure: E2E Tests"}]\n' > "$work/issues.json"
printf '{"body":"Scheduled run ended: https://github.com/Owner/Repo/actions/runs/12345\\n\\nFailed..."}\n' > "$work/issue.json"
scenario "run linked from the issue body -> reported" "$REPORTED" 0 \
  "reported=true" "reported=false" nomutate

seed
printf '[{"number":42,"title":"Nightly failure: E2E Tests"}]\n' > "$work/issues.json"
printf '[{"body":"older run https://github.com/Owner/Repo/actions/runs/999"},{"body":"https://github.com/Owner/Repo/actions/runs/12345"}]\n' > "$work/comments.json"
scenario "run linked from a comment -> reported" "$REPORTED" 0 \
  "reported=true" "reported=false" nomutate

seed
printf '[{"number":42,"title":"Nightly failure: E2E Tests"}]\n' > "$work/issues.json"
printf '{"body":"https://github.com/Owner/Repo/actions/runs/123456"}\n' > "$work/issue.json"
printf '[{"body":"https://github.com/Owner/Repo/actions/runs/1234"}]\n' > "$work/comments.json"
scenario "a longer/shorter run id is not this run" "$REPORTED" 0 \
  "reported=false" "reported=true" nomutate

seed
printf '[%s]\n' "$near_misses" > "$work/issues.json"
printf '{"body":"https://github.com/Owner/Repo/actions/runs/12345"}\n' > "$work/issue.json"
scenario "a near-miss issue linking the run does not count" "$REPORTED" 0 \
  "reported=false" "reported=true" nomutate

echo
echo "nightly-alert tests: ${pass} passed, ${fail} failed"
[ "$fail" -eq 0 ]
