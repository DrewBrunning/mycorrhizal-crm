#!/usr/bin/env bash
#
# Tests for ../manual-gate-facts.sh (issue #1486), against a throwaway git repo.
# What each case pins: an ancestor with later changes lists exactly those paths,
# an unchanged ancestor lists none, a commit from a diverged branch or one the
# checkout has never seen is a NON-ancestor (so the attestation fails closed),
# and a bad release commit / no arguments is a usage error.

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$SCRIPT_DIR/../manual-gate-facts.sh"

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

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
cd "$tmp" || exit 1
git init -q .
git config user.email t@example.invalid
git config user.name t
git config commit.gpgsign false

mkdir -p backend/embedded android/app
echo one > backend/embedded/server.go
echo one > android/app/build.gradle.kts
git add -A && git commit -q -m c1
c1="$(git rev-parse HEAD)"

echo two > backend/embedded/server.go
mkdir -p docs && echo x > docs/a.md
git add -A && git commit -q -m c2
c2="$(git rev-parse HEAD)"

git checkout -q -b side "$c1"
echo side > side.txt
git add -A && git commit -q -m side
side="$(git rev-parse HEAD)"
git checkout -q -

# 1. ancestor with later changes lists exactly the changed paths
out="$(bash "$SCRIPT" "$c2" "$c1")"
ok=1
[ "$(printf '%s' "$out" | jq -r --arg s "$c1" '.[$s].ancestor')" = "true" ] || ok=0
[ "$(printf '%s' "$out" | jq -c --arg s "$c1" '.[$s].changed | sort')" = '["backend/embedded/server.go","docs/a.md"]' ] || ok=0
check "ancestor lists the paths changed since it" "$ok" "$out"

# 2. the release commit itself is an ancestor with no changes
out="$(bash "$SCRIPT" "$c2" "$c2")"
ok=1
[ "$(printf '%s' "$out" | jq -r --arg s "$c2" '.[$s].ancestor')" = "true" ] || ok=0
[ "$(printf '%s' "$out" | jq -c --arg s "$c2" '.[$s].changed')" = '[]' ] || ok=0
check "the release commit itself: ancestor, nothing changed" "$ok" "$out"

# 3. a commit on a diverged branch is not an ancestor
out="$(bash "$SCRIPT" "$c2" "$side")"
ok=1
[ "$(printf '%s' "$out" | jq -r --arg s "$side" '.[$s].ancestor')" = "false" ] || ok=0
[ "$(printf '%s' "$out" | jq -c --arg s "$side" '.[$s].changed')" = '[]' ] || ok=0
check "diverged-branch commit is a non-ancestor" "$ok" "$out"

# 4. a commit this checkout has never seen is a non-ancestor, not an error
ghost="0123456789012345678901234567890123456789"
out="$(bash "$SCRIPT" "$c2" "$ghost")"
status=$?
ok=1
[ "$status" -eq 0 ] || ok=0
[ "$(printf '%s' "$out" | jq -r --arg s "$ghost" '.[$s].ancestor')" = "false" ] || ok=0
check "unknown commit fails closed as a non-ancestor" "$ok" "$out"

# 5. several commits in one call
out="$(bash "$SCRIPT" "$c2" "$c1" "$ghost")"
check "multiple commits keyed independently" "$([ "$(printf '%s' "$out" | jq 'keys | length')" = 2 ] && echo 1 || echo 0)" "$out"

# 6. no attested commits: an empty object
out="$(bash "$SCRIPT" "$c2")"
check "no attested commits yields {}" "$([ "$out" = '{}' ] && echo 1 || echo 0)" "$out"

# 7. usage errors exit 2
bash "$SCRIPT" >/dev/null 2>&1
check "no arguments is a usage error (exit 2)" "$([ $? -eq 2 ] && echo 1 || echo 0)"
bash "$SCRIPT" "$ghost" "$c1" >/dev/null 2>&1
check "unknown release commit is a usage error (exit 2)" "$([ $? -eq 2 ] && echo 1 || echo 0)"

echo
echo "passed=$pass failed=$fail"
[ "$fail" -eq 0 ]
