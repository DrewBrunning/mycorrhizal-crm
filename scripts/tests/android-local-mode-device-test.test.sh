#!/usr/bin/env bash
#
# Tests for ../android-local-mode-device-test.sh (issue #1486), via its --junit
# testing hook (no Gradle, no device) in a throwaway git repo. What they pin:
# a SKIPPED run is never a pass (the exact trap the gate exists for), nor a
# failed run, a run of the wrong test class, a non-arm64 device or a dirty tree;
# a good run with --record appends a complete attestation naming HEAD and
# retains the XML under the digest it records; without --record the ledger is
# untouched.

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$SCRIPT_DIR/../android-local-mode-device-test.sh"
LEDGER_SRC="$SCRIPT_DIR/../../.github/manual-gates.json"

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
mkdir "$tmp/repo"
cd "$tmp/repo" || exit 1
git init -q .
git config user.email t@example.invalid
git config user.name "Test Attester"
git config commit.gpgsign false
mkdir -p .github
cp "$LEDGER_SRC" .github/manual-gates.json
git add -A && git commit -q -m init
head_sha="$(git rev-parse HEAD)"

suite() { # tests skipped failures errors classname
  printf '<?xml version="1.0" encoding="UTF-8"?>\n<testsuite name="Pixel 8a - 15" tests="%s" failures="%s" errors="%s" skipped="%s" time="1.0"><testcase name="healthOverSocket" classname="%s"/></testsuite>\n' "$1" "$3" "$4" "$2" "$5"
}
good="$tmp/good.xml"
suite 1 0 0 0 com.mycorrhizal.crm.e2e.LocalOnlyModeE2eTest > "$good"
cls="com.mycorrhizal.crm.e2e.LocalOnlyModeE2eTest"

run() { bash "$SCRIPT" "$@" >"$tmp/out" 2>&1; echo $?; }
ledger_before="$(cat .github/manual-gates.json)"

# 1. good run without --record: success, ledger untouched
rc="$(run --junit "$good" --device "Pixel 8a" --abi arm64-v8a)"
check "a ran-and-passed run succeeds" "$(yes "$rc" 0)" "rc=$rc $(cat "$tmp/out")"
check "without --record the ledger is untouched" "$(yes "$(cat .github/manual-gates.json)" "$ledger_before")"

# 2. skipped is NOT a pass
suite 1 1 0 0 "$cls" > "$tmp/skipped.xml"
rc="$(run --junit "$tmp/skipped.xml" --device "Pixel 8a" --abi arm64-v8a)"
check "a SKIPPED run fails" "$(yes "$rc" 1)" "rc=$rc"
check "the skip message explains why a skip is no evidence" "$(grep -q 'a skip is not evidence' "$tmp/out" && echo 1 || echo 0)"

# 3. failed / errored
suite 1 0 1 0 "$cls" > "$tmp/failed.xml"
rc="$(run --junit "$tmp/failed.xml" --device "Pixel 8a" --abi arm64-v8a)"
check "a failed run fails" "$(yes "$rc" 1)"
suite 1 0 0 1 "$cls" > "$tmp/errored.xml"
rc="$(run --junit "$tmp/errored.xml" --device "Pixel 8a" --abi arm64-v8a)"
check "an errored run fails" "$(yes "$rc" 1)"

# 4. zero tests
suite 0 0 0 0 "$cls" > "$tmp/empty.xml"
rc="$(run --junit "$tmp/empty.xml" --device "Pixel 8a" --abi arm64-v8a)"
check "a run that executed nothing fails" "$(yes "$rc" 1)"

# 5. wrong test class
suite 1 0 0 0 com.example.SomethingElse > "$tmp/other.xml"
rc="$(run --junit "$tmp/other.xml" --device "Pixel 8a" --abi arm64-v8a)"
check "a run of some other test class fails" "$(yes "$rc" 1)"

# 6. wrong ABI
rc="$(run --junit "$good" --device "Pixel Emulator" --abi x86_64)"
check "a non-arm64 device fails" "$(yes "$rc" 1)"

# 7. no XML suite
echo '<nothing/>' > "$tmp/nosuite.xml"
rc="$(run --junit "$tmp/nosuite.xml" --device "Pixel 8a" --abi arm64-v8a)"
check "an XML with no testsuite fails" "$(yes "$rc" 1)"

# 8. usage errors
rc="$(run --junit "$tmp/missing.xml" --device d --abi arm64-v8a)"
check "a missing JUnit file is a usage error" "$(yes "$rc" 2)"
rc="$(run --junit "$good")"
check "--junit without --device/--abi is a usage error" "$(yes "$rc" 2)"
rc="$(run --bogus)"
check "an unknown option is a usage error" "$(yes "$rc" 2)"

# 9. a testsuites wrapper is summed
printf '<testsuites><testsuite name="a" tests="2" failures="0" errors="0" skipped="0"/><testsuite name="b" tests="1" failures="0" errors="0" skipped="0"><testcase classname="%s"/></testsuite></testsuites>\n' "$cls" > "$tmp/wrap.xml"
rc="$(run --junit "$tmp/wrap.xml" --device "Pixel 8a" --abi arm64-v8a)"
check "a <testsuites> wrapper is summed" "$(yes "$rc" 0)" "$(cat "$tmp/out")"
check "...to tests=3" "$(grep -q 'tests=3 ' "$tmp/out" && echo 1 || echo 0)" "$(cat "$tmp/out")"

# 10. --record appends a complete attestation naming HEAD and retains the XML
rc="$(run --junit "$good" --device "Pixel 8a" --abi arm64-v8a --record)"
check "--record succeeds" "$(yes "$rc" 0)" "rc=$rc $(cat "$tmp/out")"
att="$(jq -c '.gates[0].attestations[0]' .github/manual-gates.json)"
check "the attestation names HEAD" "$(yes "$(printf '%s' "$att" | jq -r .commit)" "$head_sha")" "$att"
check "the attestation records the attester from git config" "$(yes "$(printf '%s' "$att" | jq -r .attester)" "Test Attester")"
check "the attestation records tests/skipped/failures/abi/device" "$(yes "$(printf '%s' "$att" | jq -c '[.tests,.skipped,.failures,.abi,.device]')" '[1,0,0,"arm64-v8a","Pixel 8a"]')" "$att"
ev="$(printf '%s' "$att" | jq -r .evidence)"
check "the evidence XML is retained under .github/manual-gates-evidence/" "$([ -f "$ev" ] && case "$ev" in .github/manual-gates-evidence/*) echo 1;; *) echo 0;; esac || echo 0)" "$ev"
check "junit_sha256 is the retained file's digest" "$(yes "$(printf '%s' "$att" | jq -r .junit_sha256)" "$(sha256sum "$ev" | cut -d' ' -f1)")"
check "the attestation date is today (UTC)" "$(yes "$(printf '%s' "$att" | jq -r .date)" "$(date -u +%Y-%m-%d)")"

# 11. a dirty tracked tree is refused (the attestation names HEAD)
git add -A && git commit -q -m recorded
echo drift >> .github/manual-gates.json
rc="$(run --junit "$good" --device "Pixel 8a" --abi arm64-v8a)"
check "uncommitted tracked changes are refused" "$(yes "$rc" 2)" "$(cat "$tmp/out")"
git checkout -q -- .github/manual-gates.json

# 12. a second --record appends rather than replaces
git commit -q --allow-empty -m next
rc="$(run --junit "$good" --device "Pixel 8a" --abi arm64-v8a --record --attester "Someone Else")"
check "a second --record appends" "$(yes "$(jq '.gates[0].attestations | length' .github/manual-gates.json)" 2)" "rc=$rc $(cat "$tmp/out")"

echo
echo "passed=$pass failed=$fail"
[ "$fail" -eq 0 ]
