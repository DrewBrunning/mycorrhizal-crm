#!/usr/bin/env bash
#
# Tests for ../codecov-stub/post-noop-statuses.sh (issue #1188 follow-up).
#
# Pins: the PR is found by head owner:branch (never by the run's empty
# pull_requests array), a stale head posts nothing, a "no-upload" verdict
# posts exactly one success status per area, an "upload" verdict posts
# nothing, dry-run posts nothing, and renames are fed to the verdict under
# both names. `gh` and the codecovstub binary are stubs on PATH.

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$SCRIPT_DIR/../codecov-stub/post-noop-statuses.sh"

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
  if [ "$prev" = "--jq" ]; then jqfilter="$a"; fi
  prev="$a"
done
args="$*"
case "$args" in
  *"method POST"*) printf 'MUTATE: %s\n' "$args" >> "$GH_LOG"; resp='{}' ;;
  *"/pulls/"*"/files"*) resp="$(cat "$FILES_JSON")" ;;
  *"/pulls/"*) resp="$(cat "$PR_JSON")" ;;
  *"/pulls"*) resp="$(cat "$PULLS_JSON")" ;;
  *) resp='{}' ;;
esac
if [ -n "$jqfilter" ]; then printf "%s" "$resp" | jq -r "$jqfilter"; else printf "%s\n" "$resp"; fi
STUB
chmod +x "$work/bin/gh"

# The verdict binary: records its stdin, prints $VERDICT.
cat > "$work/stubbin" <<'STUB'
#!/usr/bin/env bash
cat > "$STUB_STDIN"
printf '%s\n' "$VERDICT"
STUB
chmod +x "$work/stubbin"

seed() {
  printf '[{"number":2402,"head":{"sha":"abc123"}}]\n' > "$work/pulls.json"
  printf '{"head":{"sha":"abc123"}}\n' > "$work/pr.json"
  printf '[{"filename":"CHANGELOG.md"},{"filename":"docs/new.md","previous_filename":"backend/old.go"}]\n' > "$work/files.json"
}
seed

# scenario NAME WANT_RC WANT_POSTS WANT_OUT_SUBSTR -- ENV=VAL...
scenario() {
  local name="$1" want_rc="$2" want_posts="$3" want_out="$4"
  shift 4
  [ "${1:-}" = "--" ] && shift
  : > "$work/gh.log"
  local out rc
  out="$(env PATH="$work/bin:$PATH" GH_LOG="$work/gh.log" GH_REPO="Owner/Repo" GH_TOKEN="x" \
    HEAD_SHA="abc123" HEAD_OWNER="forker" HEAD_BRANCH="feat/x" \
    PATCH_AREAS="backend frontend android" RUN_URL="https://example.test/run/1" \
    CODECOVSTUB_BIN="$work/stubbin" FILTERS_FILE="$work/filters.yaml" \
    VERDICT="no-upload" STUB_STDIN="$work/stdin.txt" \
    PULLS_JSON="$work/pulls.json" PR_JSON="$work/pr.json" FILES_JSON="$work/files.json" \
    "$@" bash "$SCRIPT" 2>&1)"
  rc=$?
  local posts
  posts="$(grep -c '^MUTATE' "$work/gh.log")"
  local ok=1 reason=""
  [ "$rc" -eq "$want_rc" ] || { ok=0; reason="rc=$rc want=$want_rc"; }
  [ "$posts" -eq "$want_posts" ] || { ok=0; reason="posts=$posts want=$want_posts"; }
  if [ -n "$want_out" ] && ! grep -qF -- "$want_out" <<<"$out"; then ok=0; reason="missing output '$want_out'"; fi
  if [ "$ok" -eq 1 ]; then pass=$((pass + 1)); echo "ok   - $name"; else fail=$((fail + 1)); echo "FAIL - $name: $reason"; echo "$out"; fi
}

scenario "no-upload verdict posts one success per area" 0 3 "verdict: no-upload"
expect() {
  local name="$1"
  shift
  if "$@"; then pass=$((pass + 1)); echo "ok   - $name"; else fail=$((fail + 1)); echo "FAIL - $name"; fi
}
logged() { grep -q -- "$1" "$work/gh.log"; }
expect "statuses name every area context" logged 'context=codecov/patch/android'
expect "statuses target the PR head sha" logged 'statuses/abc123'
expect "PR is found by head owner:branch" logged 'head=forker:feat/x'
renames_ok() { [ "$(LC_ALL=C sort "$work/stdin.txt" | tr '\n' ' ')" = "CHANGELOG.md backend/old.go docs/new.md " ]; }
expect "renames reach the verdict under both names" renames_ok

scenario "upload verdict posts nothing" 0 0 "Codecov will report" -- VERDICT=upload
scenario "dry run posts nothing" 0 0 "DRY RUN: would POST" -- DRY_RUN=1

printf '[{"number":2402,"head":{"sha":"newer"}}]\n' > "$work/pulls.json"
scenario "stale head (branch moved on) posts nothing" 0 0 "nothing to post"
seed
printf '[]\n' > "$work/pulls.json"
scenario "no open PR posts nothing" 0 0 "nothing to post"
seed

scenario "manual dispatch reads head sha from the PR" 0 3 "verdict: no-upload" -- PR_NUMBER=2402 HEAD_SHA= HEAD_OWNER= HEAD_BRANCH=
scenario "missing filters file env is an error" 1 0 "FILTERS_FILE required" -- FILTERS_FILE=

echo
echo "$pass passed, $fail failed"
[ "$fail" -eq 0 ]
