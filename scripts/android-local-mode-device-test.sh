#!/usr/bin/env bash
#
# Run LocalOnlyModeE2eTest on a real arm64 device and, with --record, write the
# result into the manual-gate attestation ledger (issue #1486, #1339).
#
# Why this exists: the embedded Go server ships arm64-only, so the test SKIPS on
# the x86_64 CI emulator and the green `Android E2E (emulator)` check proves
# nothing about local mode (docs/development/release-gates.md, "Manual gates").
# This is the one command that runs it the way the runbook demands -- embedded
# server built, a suffixed debug appId so `connectedAndroidTest`'s uninstall can
# never touch the real app and its data -- and refuses to call a SKIP a pass.
#
# Usage (repo root):
#
#   scripts/android-local-mode-device-test.sh [--record] [--serial SERIAL]
#                                             [--app-id-suffix .devicetest]
#                                             [--attester NAME]
#
#   --record      append a verifiable attestation to .github/manual-gates.json
#                 and retain the JUnit XML under .github/manual-gates-evidence/;
#                 commit both (the commit is the attestation release.yml reads).
#
# Without --record it runs and reports only. Requires: adb, a USB/wifi arm64
# device, a Go toolchain (the embedded server is cross-compiled), jq, and a
# CLEAN tracked tree -- the attestation names HEAD, so uncommitted changes
# would attest code nobody can check out.
#
# Testing hook (used only by scripts/tests/android-local-mode-device-test.test.sh):
#   --junit FILE --device NAME --abi ABI   evaluate an existing JUnit XML instead
#                                          of driving Gradle and a device.
#
# Exit 0: the test RAN on arm64 and passed (and was recorded, with --record).
# Exit 1: it skipped, failed, or ran on the wrong ABI. Exit 2: usage/environment.

set -eu

GATE_ID="android-local-mode-device"
TEST_CLASS="com.mycorrhizal.crm.e2e.LocalOnlyModeE2eTest"
MARKER="LocalOnlyModeE2eTest"
LEDGER=".github/manual-gates.json"
EVIDENCE_DIR=".github/manual-gates-evidence/${GATE_ID}"
REQUIRED_ABI="arm64-v8a"

record=false
serial=""
suffix=".devicetest"
attester=""
junit=""
device=""
abi=""

while [ "$#" -gt 0 ]; do
  case "$1" in
    --record) record=true; shift ;;
    --serial) serial="${2:-}"; shift 2 ;;
    --app-id-suffix) suffix="${2:-}"; shift 2 ;;
    --attester) attester="${2:-}"; shift 2 ;;
    --junit) junit="${2:-}"; shift 2 ;;
    --device) device="${2:-}"; shift 2 ;;
    --abi) abi="${2:-}"; shift 2 ;;
    *) echo "android-local-mode-device-test.sh: unknown option: $1" >&2; exit 2 ;;
  esac
done

die() { echo "android-local-mode-device-test.sh: $1" >&2; exit "${2:-2}"; }

command -v jq >/dev/null 2>&1 || die "jq is required"
[ -f "$LEDGER" ] || die "run from the repository root ($LEDGER not found)"

head_sha="$(git rev-parse HEAD)"
if [ -n "$(git status --porcelain --untracked-files=no)" ]; then
  die "tracked files have uncommitted changes; the attestation names HEAD (${head_sha}), so commit or stash first"
fi

if [ -z "$junit" ]; then
  command -v adb >/dev/null 2>&1 || die "adb is required"
  if [ -z "$serial" ]; then
    n="$(adb devices | awk 'NR>1 && $2=="device"' | wc -l)"
    [ "$n" -eq 1 ] || die "expected exactly one attached device, found ${n}; pass --serial"
    serial="$(adb devices | awk 'NR>1 && $2=="device"{print $1}')"
  fi
  export ANDROID_SERIAL="$serial"
  abi="$(adb -s "$serial" shell getprop ro.product.cpu.abi | tr -d '\r')"
  device="$(adb -s "$serial" shell getprop ro.product.model | tr -d '\r')"
  [ "$abi" = "$REQUIRED_ABI" ] || die "device ${serial} (${device}) is ${abi}; the embedded server runs on ${REQUIRED_ABI} only" 1

  # The suffixed appId keeps `connectedAndroidTest`'s uninstall away from the
  # production package com.mycorrhizal.crm and its on-device data.
  [ -n "$suffix" ] || die "--app-id-suffix must not be empty (it protects the real app's data)"
  results_dir="android/app/build/outputs/androidTest-results"
  rm -rf "$results_dir"
  (
    cd android
    ./gradlew :app:connectedObtainiumDebugAndroidTest \
      -PMYCORRHIZAL_BUILD_EMBEDDED_SERVER=true \
      -PMYCORRHIZAL_APP_ID_SUFFIX="$suffix" \
      "-Pandroid.testInstrumentationRunnerArguments.class=${TEST_CLASS}"
  ) || die "Gradle failed -- the test did not pass" 1
  junit="$(find "$results_dir" -name 'TEST-*.xml' | head -n 1)"
  [ -n "$junit" ] || die "Gradle produced no JUnit XML under ${results_dir}" 1
else
  [ -f "$junit" ] || die "--junit file not found: $junit"
  [ -n "$device" ] && [ -n "$abi" ] || die "--junit needs --device and --abi"
fi

# Sum every <testsuite ...> tag's counts (works for a <testsuites> wrapper too).
suites="$(grep -o '<testsuite [^>]*>' "$junit" || true)"
[ -n "$suites" ] || die "$junit has no <testsuite> element" 1
sum_attr() {
  printf '%s\n' "$suites" | grep -o "$1=\"[0-9]*\"" | sed 's/[^0-9]//g' | awk '{s+=$1} END {print s+0}'
}
tests="$(sum_attr tests)"
skipped="$(sum_attr skipped)"
failures=$(( $(sum_attr failures) + $(sum_attr errors) ))

echo "JUnit: tests=${tests} skipped=${skipped} failures=${failures} abi=${abi} device=${device}"
grep -q "$MARKER" "$junit" || die "$junit never mentions ${MARKER}; wrong test class" 1
[ "$tests" -ge 1 ] || die "the run executed no tests" 1
[ "$skipped" -eq 0 ] || die "the test SKIPPED (${skipped}); a skip is not evidence. Is libmycorrhizal.so packaged (-PMYCORRHIZAL_BUILD_EMBEDDED_SERVER=true) and the device arm64?" 1
[ "$failures" -eq 0 ] || die "the test FAILED (${failures})" 1
[ "$abi" = "$REQUIRED_ABI" ] || die "ran on ${abi}, not ${REQUIRED_ABI}" 1

echo "LocalOnlyModeE2eTest RAN and passed on ${device} (${abi})."

if [ "$record" != "true" ]; then
  echo "Not recorded (pass --record to write the attestation into ${LEDGER})."
  exit 0
fi

[ -n "$attester" ] || attester="$(git config user.name || true)"
[ -n "$attester" ] || die "no attester: set git user.name or pass --attester"

date_utc="$(date -u +%Y-%m-%d)"
sha256="$(sha256sum "$junit" | cut -d' ' -f1)"
mkdir -p "$EVIDENCE_DIR"
evidence="${EVIDENCE_DIR}/${date_utc}-${head_sha:0:7}.xml"
cp "$junit" "$evidence"

tmp="$(mktemp)"
jq --arg id "$GATE_ID" --arg date "$date_utc" --arg commit "$head_sha" --arg attester "$attester" \
  --arg evidence "$evidence" --arg device "$device" --arg abi "$abi" \
  --argjson tests "$tests" --argjson skipped "$skipped" --argjson failures "$failures" --arg sha "$sha256" \
  '(.gates[] | select(.id == $id) | .attestations) += [{date: $date, commit: $commit, attester: $attester, evidence: $evidence, device: $device, abi: $abi, tests: $tests, skipped: $skipped, failures: $failures, junit_sha256: $sha}]' \
  "$LEDGER" > "$tmp"
mv "$tmp" "$LEDGER"

echo "Recorded in ${LEDGER}; evidence retained at ${evidence}."
echo "Commit both (git commit -s). release.yml's attest_manual_gates=${GATE_ID} then verifies it: <= 14 days old, ${head_sha:0:12} an ancestor of the release commit, no watched path changed since."
