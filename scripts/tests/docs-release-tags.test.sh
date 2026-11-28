#!/usr/bin/env bash
#
# Tests for ../docs-release-tags.sh (issue #2244): only release tags on a
# reviewed line of history are published as docs. A `v*` tag on main or on a
# release/* branch is listed; a tag pushed on any other commit (an unmerged
# branch) is not, so it can never become the docs site root.

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$SCRIPT_DIR/../docs-release-tags.sh"

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
listed() { grep -qx "$1" "$tmp/out" && echo 1 || echo 0; }
absent() { grep -qx "$1" "$tmp/out" && echo 0 || echo 1; }

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
# A hook exports GIT_DIR and friends; without this the fixture repo's commands
# would act on the real repository.
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_PREFIX GIT_COMMON_DIR
repo="$tmp/repo"
g() { git -C "$repo" -c user.name=t -c user.email=t@example.invalid -c commit.gpgsign=false -c tag.gpgsign=false "$@"; }
mkdir -p "$repo"
g init -q -b main .
g commit -q --allow-empty -m base
g tag v1.6.0
g commit -q --allow-empty -m "release 1.7.0"
g tag -a v1.7.0 -m "annotated release tag"
g tag not-a-release

# A release branch (security backports land here before tagging), known only
# as a remote-tracking ref, as in a CI checkout.
g checkout -q -b release/v1.6.0 v1.6.0
g commit -q --allow-empty -m "backport"
g tag v1.6.1
g update-ref refs/remotes/origin/release/v1.6.0 HEAD
g checkout -q main
g branch -q -D release/v1.6.0

# A local release branch (a developer checkout).
g checkout -q -b release/v1.5.0 v1.6.0
g commit -q --allow-empty -m "local backport"
g tag v1.5.9

# An unmerged feature branch: anyone who can create a tag could push this one.
g checkout -q -b feature/evil main
g commit -q --allow-empty -m "unreviewed"
g tag v9.9.9
g checkout -q main

( cd "$repo" && bash "$SCRIPT" main ) >"$tmp/out" 2>"$tmp/err"
rc=$?
check "exits 0" "$([ "$rc" = 0 ] && echo 1 || echo 0)" "rc=$rc $(cat "$tmp/err")"
check "lists a lightweight tag on main" "$(listed v1.6.0)" "$(cat "$tmp/out")"
check "lists an annotated tag on main" "$(listed v1.7.0)" "$(cat "$tmp/out")"
check "lists a tag on an origin/release/* branch" "$(listed v1.6.1)" "$(cat "$tmp/out")"
check "lists a tag on a local release/* branch" "$(listed v1.5.9)" "$(cat "$tmp/out")"
check "skips a tag on an unmerged branch" "$(absent v9.9.9)" "$(cat "$tmp/out")"
check "skips a tag that is not v*" "$(absent not-a-release)" "$(cat "$tmp/out")"
check "lists each tag once" "$([ "$(sort "$tmp/out" | uniq -d | wc -l)" = 0 ] && echo 1 || echo 0)" "$(cat "$tmp/out")"

# The ref defaults to HEAD.
( cd "$repo" && bash "$SCRIPT" ) >"$tmp/out" 2>&1
check "defaults to HEAD" "$(listed v1.7.0)" "$(cat "$tmp/out")"

# A ref that does not exist is an error, never an empty (all-unpublished) site.
( cd "$repo" && bash "$SCRIPT" no-such-ref ) >"$tmp/out" 2>&1
rc=$?
check "fails on an unknown ref" "$([ "$rc" != 0 ] && echo 1 || echo 0)" "rc=$rc"

echo "$pass passed, $fail failed"
[ "$fail" = 0 ]
