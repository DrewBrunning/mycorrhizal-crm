#!/usr/bin/env bash
# Issue #1487: `go test` with a bounded, loud retry of the FAILED PACKAGES only.
#
# Data (release.yml runs, 2026-09-19..22): min-version-tests' Go-floor leg
# (`go test ./...` at the declared 1.26.0 floor, no -race) failed three times,
# each time in a different, timing-sensitive test -- a SQLITE_BUSY lock-release
# race in services' db-integrity webhook test (twice, `services` also takes
# ~2-3 minutes under a contended runner), a reach-out suggestion test, and a
# schemafixture 10m timeout since raised to 25m. None was floor-specific: the
# same packages pass at the current toolchain in the same battery. The leg's
# purpose is "does the floor still build and pass", so a lone timing flake in
# one package must not cost a whole release cut -- but a floor regression must
# still fail, so this is not a blanket retry:
#
#   * a build/vet failure (`[build failed]`) is deterministic -> fail, no retry;
#   * more than $GO_TEST_RETRY_MAX_PKGS failed packages (default 3) is a real
#     regression, not a flake -> fail, no retry;
#   * otherwise ONLY the failed packages re-run once with -count=1; if they pass
#     the leg passes with a ::warning:: naming each retried package and test, so
#     the flake stays visible; if any fails again the leg fails.
#
# Env: GO_TEST_FLAGS (e.g. "-timeout 25m"), GO_TEST_PKGS (default ./...),
#      GO_TEST_RETRY_MAX_PKGS (default 3). Run from the module directory.
set -uo pipefail

flags="${GO_TEST_FLAGS:-}"
pkgs="${GO_TEST_PKGS:-./...}"
max_pkgs="${GO_TEST_RETRY_MAX_PKGS:-3}"

out="$(mktemp)"
trap 'rm -f "$out"' EXIT

# shellcheck disable=SC2086 # flags/pkgs are deliberately word-split
go test $flags $pkgs 2>&1 | tee "$out"
rc="${PIPESTATUS[0]}"
if [ "$rc" -eq 0 ]; then
  exit 0
fi

if grep -qE '^FAIL[[:space:]].*\[(build|setup) failed\]' "$out"; then
  echo "::error::go test failed to build/set up a package -- deterministic, not retried"
  exit "$rc"
fi

mapfile -t failed < <(grep -E '^FAIL[[:space:]]+[^[:space:]]+[[:space:]]+[0-9.]+s' "$out" | awk '{print $2}' | sort -u)
if [ "${#failed[@]}" -eq 0 ]; then
  echo "::error::go test failed with no identifiable failed package -- not retried"
  exit "$rc"
fi
if [ "${#failed[@]}" -gt "$max_pkgs" ]; then
  echo "::error::${#failed[@]} packages failed (more than ${max_pkgs}): a regression, not a flake -- not retried"
  exit "$rc"
fi

tests="$(grep -E '^--- FAIL: ' "$out" | awk '{print $3}' | sort -u | paste -sd, -)"
echo "::warning::retrying once the ${#failed[@]} failed package(s): ${failed[*]} (failed tests: ${tests:-none recorded, e.g. a timeout}) (issue #1487)"

# shellcheck disable=SC2086
if go test $flags -count=1 "${failed[@]}"; then
  echo "::warning::FLAKE: ${failed[*]} failed once and passed on retry (tests: ${tests:-none recorded}). Triage; the leg is not blocked (issue #1487)."
  exit 0
fi
echo "::error::${failed[*]} failed again on retry -- a real failure at the floor"
exit 1
