#!/usr/bin/env bash
#
# Tests for ../pr-alert/ensure-comment.sh (issue #1964).
#
# Pins the contract the workflow relies on: one marker-tagged comment per PR,
# updated in place on a repeated failure (never duplicated), the failing job
# name and its log URL in the body, the local repro command, and no mutation
# in dry-run mode. `gh` is a stub on PATH that records mutating calls.

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$SCRIPT_DIR/../pr-alert/ensure-comment.sh"

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
  *"actions/runs/${RUN_ID}/jobs"*) resp="$(cat "$JOBS_JSON")" ;;
  *"issues/${PR_NUMBER}/comments"*) resp="$(cat "$COMMENTS_JSON")" ;;
  *"commits/"*"/pulls"*) resp="$(cat "$PULLS_JSON")" ;;
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
  printf '{"jobs":[{"name":"Backend (Go)","conclusion":"failure","html_url":"https://example.test/job/7"}]}\n' > "$work/jobs.json"
  printf '[{"number":123,"state":"open"}]\n' > "$work/pulls.json"
  printf '[]\n' > "$work/comments.json"
}
seed

# scenario NAME WANT_RC WANT_ABSENT MUTATION -- ENV=VAL...
scenario() {
  local name="$1" want_rc="$2" absent="$3" mutation="$4"
  shift 4
  [ "${1:-}" = "--" ] && shift
  : > "$work/gh.log"
  local out rc
  out="$(env PATH="$work/bin:$PATH" \
    GH_LOG="$work/gh.log" GH_REPO="Owner/Repo" GH_TOKEN="x" \
    WORKFLOW_NAME="Unit Tests" RUN_ID="1000" RUN_URL="https://example.test/run/1000" \
    CONCLUSION="failure" RUN_ATTEMPT="1" HEAD_SHA="deadbeef" PR_NUMBER="123" \
    JOBS_JSON="$work/jobs.json" PULLS_JSON="$work/pulls.json" COMMENTS_JSON="$work/comments.json" \
    "$@" bash "$SCRIPT" 2>&1)"
  rc=$?

  local ok=1 reason=""
  if [ "$rc" -ne "$want_rc" ]; then ok=0; reason="rc=$rc want=$want_rc"; fi
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

  # Every scenario whose run has a failed job must carry the marker, the job,
  # its log URL, and the local repro command.
  if [ "$ok" -eq 1 ] && [ "$mutation" = "mutate" ] && grep -qF 'Backend (Go)' "$work/jobs.json"; then
    for want in '<!-- pr-failure-alert -->' 'Backend (Go)' 'https://example.test/job/7' 'scripts/pre-pr.sh --focused'; do
      if ! grep -qF -- "$want" "$work/gh.log"; then
        ok=0; reason="mutation body missing '$want'"
      fi
    done
  fi

  if [ "$ok" -eq 1 ]; then
    pass=$((pass + 1))
    echo "PASS: $name"
  else
    fail=$((fail + 1))
    echo "FAIL: $name ($reason)"
    printf '    | %s\n' "${out//$'\n'/$'\n    | '}"
  fi
}

seed
scenario "dry run logs the comment without writing" 0 "MUTATE" nomutate -- DRY_RUN=1

seed
scenario "first failure creates one comment" 0 "" mutate -- DRY_RUN=0

seed
printf '[{"id":77,"body":"<!-- pr-failure-alert --> old"}]\n' > "$work/comments.json"
scenario "repeated failure updates the existing comment" 0 "" mutate -- DRY_RUN=0
if ! grep -q 'method PATCH .*issues/comments/77' "$work/gh.log"; then
  fail=$((fail + 1))
  echo "FAIL: repeated failure did not PATCH comment 77"
else
  pass=$((pass + 1))
  echo "PASS: repeated failure PATCHes the existing comment"
fi
if grep -q 'method POST' "$work/gh.log"; then
  fail=$((fail + 1))
  echo "FAIL: repeated failure also created a second comment"
else
  pass=$((pass + 1))
  echo "PASS: repeated failure does not create a second comment"
fi

# A run with no attributable PR is a quiet no-op.
seed
printf '[]\n' > "$work/pulls.json"
scenario "no open PR is a no-op" 0 "" nomutate -- DRY_RUN=0 PR_NUMBER=""

# A startup failure may have no failed job; the comment still explains that.
seed
printf '{"jobs":[]}\n' > "$work/jobs.json"
scenario "no failed job still posts an explanation" 0 "" mutate -- DRY_RUN=0 CONCLUSION=startup_failure

# E2E is CI-only: the comment must name that boundary and must not also
# recommend the pre-PR gate as a repro for the failing run.
seed
printf '{"jobs":[{"name":"Run E2E Tests","conclusion":"failure","html_url":"https://example.test/job/9"}]}\n' > "$work/jobs.json"
scenario "E2E failure posts the failure comment" 0 "" mutate -- DRY_RUN=0
if grep -qF 'E2E is CI-only' "$work/gh.log" && grep -qF 'run the failing Playwright spec' "$work/gh.log"; then
  pass=$((pass + 1))
  echo "PASS: E2E failure names the CI-only boundary"
else
  fail=$((fail + 1))
  echo "FAIL: E2E failure did not name the CI-only boundary"
fi
if grep -qF 'scripts/pre-pr.sh --focused' "$work/gh.log"; then
  fail=$((fail + 1))
  echo "FAIL: E2E failure still recommends the pre-PR gate as a repro"
else
  pass=$((pass + 1))
  echo "PASS: E2E failure does not claim a pre-PR repro"
fi

# Android E2E/macrobenchmark are emulator-only; the broad Android* prefix must
# not route them to the pre-PR gate.
seed
printf '{"jobs":[{"name":"Android E2E (emulator)","conclusion":"failure","html_url":"https://example.test/job/10"}]}\n' > "$work/jobs.json"
scenario "Android E2E failure names the emulator boundary" 0 "" mutate -- DRY_RUN=0
if grep -qF 'needs an emulator and is CI-only' "$work/gh.log" && ! grep -qF 'scripts/pre-pr.sh --focused' "$work/gh.log"; then
  pass=$((pass + 1))
  echo "PASS: Android E2E failure does not claim a pre-PR repro"
else
  fail=$((fail + 1))
  echo "FAIL: Android E2E failure recommends the pre-PR gate or omits the boundary"
fi

echo
echo "pr-alert: $pass passed, $fail failed"
[ "$fail" -eq 0 ]
