#!/usr/bin/env bash
#
# CI fuzz runner (issues #265, #376, #712).
#
# Runs every native Go fuzz target once per invocation with a single
# -fuzztime budget, hardened against the golang/go#75804 deadline-boundary
# race: a target can be reported as `context deadline exceeded` exactly when
# -fuzztime expires while a worker is mid-execution, even though it ran its
# full budget and found no crash. That is a coordinator reporting bug (fixed
# upstream in go1.27, CL 804900 — not backported to the 1.26 toolchain this
# repo runs), not a test failure; issue #712 observed it in the nightly at the
# 2m boundary. Softening it is safe only because the crash signature is
# unambiguous:
#
#   - "Failing input written to" in the output means a REAL crash or a
#     genuinely hung worker (verified empirically, see
#     .github/scripts/tests/run-fuzz-targets.test.sh) — the input is already
#     on disk under <pkg>/testdata/fuzz/ and replays as a deterministic
#     failure ("failure while testing seed corpus entry", which carries no
#     deadline string and therefore also lands in the hard-fail branch below).
#     This path always fails the step.
#   - "context deadline exceeded" with no crash line is the #75804 race:
#     retry once, and if it repeats, treat as a soft failure — the target DID
#     run its full budget and found nothing.
#
# Must be run from the backend/ directory (the targets are module-relative
# package paths). Usage: run-fuzz-targets.sh [FUZZTIME]; FUZZTIME defaults to
# the $FUZZTIME env var, then 15s.

set -u

FUZZTIME="${1:-${FUZZTIME:-15s}}"

targets=(
  "./vcard4 ^FuzzImportVCard4$"
  "./vcard4 ^FuzzExportVCard4$"
  "./vcard3 ^FuzzImportVCard3$"
  "./vcard3 ^FuzzExportVCard3$"
  "./jscontact ^FuzzImportJSContact$"
  "./jscontact ^FuzzExportJSContact$"
  "./services ^FuzzExtractICalEvents$"
  "./services ^FuzzParseCSV$"
)

# Run one fuzz target. Returns 0 for a clean run (or a soft-failed
# deadline-boundary race) and non-zero for a genuine crash or other failure.
run_one() {
  local dir="$1" target="$2"
  local attempt=1 log status
  while :; do
    log="$(mktemp)" || { echo "::error::cannot create temp log for $target"; return 1; }
    go test "$dir" -run '^$' -fuzz="$target" -fuzztime="$FUZZTIME" 2>&1 | tee "$log"
    status=${PIPESTATUS[0]}
    if [ "$status" -eq 0 ]; then
      rm -f "$log"
      return 0
    fi
    if grep -q "Failing input written to" "$log"; then
      echo "::error::fuzz target $target found a crash — regression input is under testdata/fuzz/ (see output above)"
      rm -f "$log"
      return "$status"
    fi
    if grep -q "context deadline exceeded" "$log"; then
      if [ "$attempt" -eq 1 ]; then
        echo "::warning::fuzz target $target hit the go fuzz deadline-boundary race (golang/go#75804, no crash found) — retrying once"
        attempt=2
        rm -f "$log"
        continue
      fi
      echo "::warning::fuzz target $target hit the deadline-boundary race again after a full retry (golang/go#75804) — no crash found, treating as a soft failure"
      rm -f "$log"
      return 0
    fi
    echo "::error::fuzz target $target failed (exit $status) — see output above"
    rm -f "$log"
    return "$status"
  done
}

failed=0
for entry in "${targets[@]}"; do
  read -r dir target <<<"$entry"
  echo "::group::fuzz $target (-fuzztime=$FUZZTIME)"
  run_one "$dir" "$target" || failed=1
  echo "::endgroup::"
done

if [ "$failed" -ne 0 ]; then
  echo "::error::one or more fuzz targets failed"
  exit 1
fi
exit 0
