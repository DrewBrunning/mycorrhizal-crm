#!/usr/bin/env bash
#
# Tests for ../bypassed-check-alert/ensure-alert.sh (issue #1661).
#
# What these pin: the verdict it receives is already settled by
# cmd/bypassedcheck's wait loop, so a failed/missing/still-incomplete context
# alerts (one issue per PR, found by exact title via the REST issues endpoint,
# never the Search API); a context that merged unfinished but later finished
# green is a job-summary line with NO issue and no mutation; an all-green
# verdict is silent.
#
# `gh` is a stub on PATH serving scripted JSON; mutating calls are recorded.

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$SCRIPT_DIR/../bypassed-check-alert/ensure-alert.sh"

indent() { printf "    | %s\n" "${1//$'\n'/$'\n    | '}"; }

pass=0
fail=0

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/bin"

cat > "$work/bin/gh" <<'STUB'
#!/usr/bin/env bash
printf 'gh %s\n' "$*" >> "$GH_LOG"
args="$*"
case "$args" in
  *"method POST"*)
    printf 'MUTATE: %s\n' "$args" >> "$GH_LOG"
    echo '{}'
    ;;
  *"label create"*) exit 0 ;;
  *"search/issues"*)
    printf 'SEARCH: %s\n' "$args" >> "$GH_LOG"
    echo '{"items":[]}'
    ;;
  *"/issues?labels=ci&"*) cat "$ISSUES_JSON" ;;
  *"/pulls/"*) echo '{"title":"Fix thing","html_url":"https://example.test/pull/7","merged_by":{"login":"drew"}}' ;;
  *) echo '{}' ;;
esac
STUB
chmod +x "$work/bin/gh"

TITLE="Merged with non-green required checks: PR #7"

# scenario NAME WANT_SUBSTRING WANT_ABSENT MUTATION -- ENV=VAL...
# MUTATION is "mutate" or "nomutate".
scenario() {
  local name="$1" want="$2" absent="$3" mutation="$4"
  shift 4
  [ "${1:-}" = "--" ] && shift
  : > "$work/gh.log"
  : > "$work/summary.md"
  local out rc
  out="$(env PATH="$work/bin:$PATH" GH_LOG="$work/gh.log" GH_REPO="Owner/Repo" GH_TOKEN="x" \
    PR_NUMBER=7 VERDICT_FILE="$work/verdict.json" ISSUES_JSON="$work/issues.json" \
    GITHUB_STEP_SUMMARY="$work/summary.md" "$@" bash "$SCRIPT" 2>&1)"
  rc=$?
  out+=$'\n'"$(cat "$work/summary.md")"

  local ok=1 reason=""
  if [ "$rc" -ne 0 ]; then ok=0; reason="rc=$rc"; fi
  if [ "$ok" -eq 1 ] && [ -n "$want" ] && ! grep -qF -- "$want" <<<"$out$(cat "$work/gh.log")"; then
    ok=0; reason="missing substring '$want'"
  fi
  if [ "$ok" -eq 1 ] && [ -n "$absent" ] && grep -qF -- "$absent" <<<"$out$(cat "$work/gh.log")"; then
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

verdict() { printf '%s\n' "$1" > "$work/verdict.json"; }
echo '[]' > "$work/issues.json"

verdict '{"sha":"abc","required":2,"findings":[],"merged_early":[],"waited_seconds":0}'
scenario "all green is silent" "every required check was green" "" nomutate

verdict '{"sha":"abc","required":2,"findings":[],"merged_early":["Backend (Go)"],"waited_seconds":360}'
# shellcheck disable=SC2016 # backticks are literal markdown
scenario "merged early but finished green: summary line, no issue" \
  'merged before `Backend (Go)` finished; it later finished green (waited 6 min)' "Opened an alert" nomutate

verdict '{"sha":"abc","required":2,"findings":[{"context":"Backend (Go)","category":"failed","detail":"failure"}],"merged_early":[],"waited_seconds":0}'
scenario "failed alerts at once" "Opened an alert issue for PR #7" "" mutate

verdict '{"sha":"abc","required":2,"findings":[{"context":"codecov/patch/backend","category":"missing","detail":"no check run or status"}],"merged_early":[],"waited_seconds":0}'
scenario "missing alerts" "never reported" "" mutate

verdict '{"sha":"abc","required":2,"findings":[{"context":"codecov/patch/backend","category":"incomplete","detail":"pending"}],"merged_early":["codecov/patch/backend"],"waited_seconds":2400}'
scenario "still incomplete after the wait alerts and says so" "still incomplete (pending) after waiting 40 min" "" mutate

verdict '{"sha":"abc","required":2,"findings":[{"context":"Backend (Go)","category":"failed","detail":"failure"}],"merged_early":["Backend (Go)"],"waited_seconds":240}'
scenario "merged early then finished red says both" "merged before it finished; it later finished red (failure)" "" mutate

# Issue lookup: REST + exact title. A near-miss title and a PR with the exact
# title must not be commented on; the exact one is.
verdict '{"sha":"abc","required":2,"findings":[{"context":"Backend (Go)","category":"failed","detail":"failure"}],"merged_early":[],"waited_seconds":0}'
printf '%s\n' "[{\"number\":50,\"title\":\"$TITLE (extra)\"},{\"number\":51,\"title\":\"$TITLE\",\"pull_request\":{}}]" > "$work/issues.json"
scenario "near-miss title and PR are ignored: opens a new issue" "Opened an alert issue" "Commented on" mutate

printf '%s\n' "[{\"number\":50,\"title\":\"$TITLE (extra)\"},{\"number\":52,\"title\":\"$TITLE\"}]" > "$work/issues.json"
scenario "exact title match is commented on" "Commented on existing issue #52" "Opened an alert" mutate

verdict '{"sha":"abc","required":2,"findings":[{"context":"Backend (Go)","category":"failed","detail":"failure"}],"merged_early":[],"waited_seconds":0}'
scenario "dry run writes nothing" "[dry-run] would ensure labels" "" nomutate -- DRY_RUN=1

echo
echo "bypassed-check-alert tests: ${pass} passed, ${fail} failed"
[ "$fail" -eq 0 ]
