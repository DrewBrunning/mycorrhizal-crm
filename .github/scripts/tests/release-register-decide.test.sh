#!/usr/bin/env bash
#
# Tests for ../release-register-decide.sh (issue #929).
#
# The bug this pins: `dry_run: true` rehearsing the last shipped version (the
# case step 1 of release.yml explicitly allows, downgrading "already
# registered" to a warning) reached the registration step with no guard of
# its own, which unconditionally appended a SECOND SupportedReleases entry
# for that already-shipped version -- corrupting the frozen chain assertion
# downstream (see the script's own header). The regression guard below is the
# case that must decide SKIP.

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$SCRIPT_DIR/../release-register-decide.sh"

pass=0
fail=0

run_test() {
  local name="$1" fixture="$2" version="$3" want="$4"

  local tmp out status ok
  tmp="$(mktemp)"
  printf '%s' "$fixture" > "$tmp"
  out="$(bash "$SCRIPT" "$tmp" "$version" 2>&1)"
  status=$?
  rm -f "$tmp"

  ok=1
  [ "$status" -eq 0 ] || ok=0
  [ "$out" = "$want" ] || ok=0

  if [ "$ok" -eq 1 ]; then
    pass=$((pass + 1))
    echo "PASS: $name"
  else
    fail=$((fail + 1))
    echo "FAIL: $name (exit=$status out=$out want=$want)"
  fi
}

fixture='var SupportedReleases = []Release{
	{Tag: "v0.6.0", Version: 31},
	{Tag: "v0.8.1", Version: 56},
	{Tag: "v0.8.2", Version: 56},
	{Tag: "v0.8.3", Version: 56},
}
'

run_test "a brand-new version not yet shipped -> REGISTER" \
  "$fixture" "v0.9.0" "REGISTER"

run_test "REGRESSION GUARD: the last shipped version (dry-run rehearsal target) -> SKIP, not a duplicate REGISTER" \
  "$fixture" "v0.8.3" "SKIP"

run_test "an earlier already-registered version -> SKIP" \
  "$fixture" "v0.6.0" "SKIP"

run_test "a version string that is a prefix of a registered tag is NOT treated as registered (comma-anchored match)" \
  "$fixture" "v0.8" "REGISTER"

run_test "a version string that only differs by patch number is NOT treated as registered" \
  "$fixture" "v0.8.30" "REGISTER"

echo ""
echo "$pass passed, $fail failed"
if [ "$fail" -ne 0 ]; then
  exit 1
fi
exit 0
