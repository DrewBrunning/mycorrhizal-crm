#!/usr/bin/env bash
#
# Tests for ../install-agent-links.sh (issues #1954/#1957): the CLAUDE.md
# aliases are created idempotently, a git-materialized .claude/skills file is
# repaired, and a real .claude/skills directory is refused rather than deleted.

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$SCRIPT_DIR/../install-agent-links.sh"

pass=0
fail=0
check() {
  local name="$1" ok="$2" detail="${3:-}"
  if [ "$ok" = 1 ]; then
    pass=$((pass + 1))
    echo "PASS: $name"
  else
    fail=$((fail + 1))
    echo "FAIL: $name ${detail}"
  fi
}
yes() { [ "$1" = "$2" ] && echo 1 || echo 0; }

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
repo="$tmp/repo"
mkdir -p "$repo"/{backend/database,frontend,android,docs,.github,.claude,.agents/skills}
for f in AGENTS.md backend/AGENTS.md frontend/AGENTS.md android/AGENTS.md docs/AGENTS.md .github/AGENTS.md backend/database/AGENTS.md; do
  printf '# context\n' > "$repo/$f"
done
git -C "$repo" init -q .

run() { ( cd "$repo" && bash "$SCRIPT" ) >"$tmp/out" 2>&1; echo $?; }

# 1. first run creates every alias
rc="$(run)"
check "first run succeeds" "$(yes "$rc" 0)" "rc=$rc $(cat "$tmp/out")"
check "skills alias is the expected symlink" "$(yes "$(readlink "$repo/.claude/skills")" "../.agents/skills")"
check "CLAUDE.md is a symlink to AGENTS.md" "$(yes "$(readlink "$repo/CLAUDE.md")" "AGENTS.md")"
check "nested CLAUDE.md is a symlink" "$(yes "$(readlink "$repo/backend/database/CLAUDE.md")" "AGENTS.md")"

# 2. re-run is idempotent
rc="$(run)"
check "second run succeeds" "$(yes "$rc" 0)" "rc=$rc"
check "second run reports the aliases already correct" "$(grep -q 'already correct' "$tmp/out" && echo 1 || echo 0)" "$(cat "$tmp/out")"

# 3. a git-materialized symlink (regular file with the target as contents) is repaired
rm "$repo/.claude/skills"
printf '../.agents/skills' > "$repo/.claude/skills"
rc="$(run)"
check "materialized skills file is repaired" "$(yes "$rc" 0)" "rc=$rc $(cat "$tmp/out")"
check "...to a real symlink" "$(yes "$(readlink "$repo/.claude/skills")" "../.agents/skills")"

# 4. a real directory is refused, not deleted
rm "$repo/.claude/skills"
mkdir "$repo/.claude/skills"
printf 'keep me\n' > "$repo/.claude/skills/keepme"
rc="$(run)"
check "a real skills directory is refused" "$(yes "$rc" 1)" "rc=$rc"
check "...and left intact" "$([ -f "$repo/.claude/skills/keepme" ] && echo 1 || echo 0)"

# 5. a regular file that is not the materialized target is refused
rm -rf "$repo/.claude/skills"
printf 'not the target\n' > "$repo/.claude/skills"
rc="$(run)"
check "an arbitrary skills file is refused" "$(yes "$rc" 1)" "rc=$rc"
check "...and left intact" "$(yes "$(cat "$repo/.claude/skills")" "not the target")"

# 6. a missing canonical tree is an error
rm -f "$repo/.claude/skills"
rm -rf "$repo/.agents"
rc="$(run)"
check "missing .agents/skills is an error" "$(yes "$rc" 1)" "rc=$rc"

echo
echo "passed=$pass failed=$fail"
[ "$fail" -eq 0 ]
