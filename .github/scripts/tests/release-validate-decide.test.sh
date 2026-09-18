#!/usr/bin/env bash
#
# Tests for ../release-validate-decide.sh (issue #1142).
#
# The regression guard this pins is the RESUME case: a final release whose
# step 7 already pushed the schema-fixture commit to main, then failed the
# step 8 release-tier wait. The old step 1 hard-refused the already-registered
# version, so the documented "fix on main and re-dispatch" recovery could
# never get past validation. The decision must now be RESUME for that state,
# while a genuinely shipped tag (already tagged) still hard-fails.

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$SCRIPT_DIR/../release-validate-decide.sh"

pass=0
fail=0

run_test() {
  local name="$1" is_rc="$2" dry_run="$3" tag_exists="$4" registered="$5" want="$6"

  local out status ok
  out="$(bash "$SCRIPT" "$is_rc" "$dry_run" "$tag_exists" "$registered" 2>&1)"
  status=$?

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

run_test "fresh final release -> PROCEED" \
  false false false false "PROCEED"

run_test "REGRESSION GUARD: final release already registered but untagged (interrupted step 8) -> RESUME" \
  false false false true "RESUME"

run_test "final release already tagged -> FAIL_TAG_EXISTS, never move a released tag" \
  false false true true "FAIL_TAG_EXISTS"

run_test "an unexpected tag with no registration still -> FAIL_TAG_EXISTS" \
  false false true false "FAIL_TAG_EXISTS"

run_test "dry run rehearsing the last shipped version (registered, untagged) -> PROCEED, not RESUME" \
  false true false true "PROCEED"

run_test "dry run targeting an already-tagged version -> PROCEED (checks downgraded)" \
  false true true true "PROCEED"

run_test "a release candidate is never itself registered -> PROCEED" \
  true false false false "PROCEED"

run_test "an RC whose tag already exists -> FAIL_TAG_EXISTS" \
  true false true false "FAIL_TAG_EXISTS"

# A missing argument must fail loudly rather than silently deciding.
out="$(bash "$SCRIPT" false false 2>&1)"
status=$?
if [ "$status" -ne 0 ]; then
  pass=$((pass + 1))
  echo "PASS: a missing argument exits non-zero"
else
  fail=$((fail + 1))
  echo "FAIL: a missing argument exited 0 (out=$out)"
fi

echo ""
echo "$pass passed, $fail failed"
if [ "$fail" -ne 0 ]; then
  exit 1
fi
exit 0
