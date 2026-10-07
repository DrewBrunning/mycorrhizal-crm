#!/usr/bin/env bash
#
# Tests for ../go-test-retry-failed.sh (issue #1487).
#
# What these pin: a lone timing flake in one package is retried (loudly) and
# does not cost the Go-floor leg, while everything that signals a real
# regression -- a build failure, many failed packages, a package that fails
# twice -- still fails the leg. `go` is a stub on PATH replaying scripted
# outputs: the first `go test` call prints $FIRST_OUT and exits $FIRST_RC, the
# retry (`-count=1`) prints $RETRY_OUT and exits $RETRY_RC.

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$SCRIPT_DIR/../go-test-retry-failed.sh"

indent() { printf "    | %s\n" "${1//$'\n'/$'\n    | '}"; }

pass=0
fail=0

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/bin"

cat > "$work/bin/go" <<'STUB'
#!/usr/bin/env bash
echo "go $*" >> "$GO_LOG"
if [[ " $* " == *" -count=1 "* ]]; then
  printf '%s\n' "${RETRY_OUT:-}"
  exit "${RETRY_RC:-0}"
fi
printf '%s\n' "${FIRST_OUT:-}"
exit "${FIRST_RC:-0}"
STUB
chmod +x "$work/bin/go"

FLAKE=$'--- FAIL: TestSomethingTimingSensitive (0.30s)\nFAIL\nFAIL\tmycorrhizal/services\t127.030s'

# scenario NAME WANT_RC WANT_GO_CALLS WANT_SUBSTRING -- ENV=VAL...
scenario() {
  local name="$1" want_rc="$2" want_calls="$3" want_out="$4"
  shift 5
  : > "$work/go.log"
  local out rc calls
  out="$(env PATH="$work/bin:$PATH" GO_LOG="$work/go.log" GO_TEST_FLAGS="-timeout 25m" "$@" bash "$SCRIPT" 2>&1)"
  rc=$?
  calls="$(wc -l < "$work/go.log")"
  if [ "$rc" -eq "$want_rc" ] && [ "$calls" -eq "$want_calls" ] && grep -qF -- "$want_out" <<<"$out"; then
    pass=$((pass + 1))
    echo "PASS: $name"
  else
    fail=$((fail + 1))
    echo "FAIL: $name (rc=$rc want=$want_rc, go calls=$calls want=$want_calls, want substring '$want_out')"
    indent "$out"
  fi
}

scenario "a green run is one go test call" 0 1 "" -- FIRST_RC=0 FIRST_OUT="ok  	mycorrhizal	1s"
scenario "one flaky package is retried alone and the leg passes with a warning" 0 2 "FLAKE: mycorrhizal/services" -- FIRST_RC=1 FIRST_OUT="$FLAKE" RETRY_RC=0
# The retry must target ONLY the failed package, with -count=1 (read from the stub's call log).
: > "$work/go.log"
env PATH="$work/bin:$PATH" GO_LOG="$work/go.log" GO_TEST_FLAGS="-timeout 25m" FIRST_RC=1 FIRST_OUT="$FLAKE" RETRY_RC=0 bash "$SCRIPT" > /dev/null 2>&1
if [ "$(sed -n 2p "$work/go.log")" = "go test -timeout 25m -count=1 mycorrhizal/services" ]; then
  pass=$((pass + 1))
  echo "PASS: the retry runs only the failed package with -count=1"
else
  fail=$((fail + 1))
  echo "FAIL: the retry runs only the failed package with -count=1"
  indent "$(cat "$work/go.log")"
fi
scenario "the warning names the failed test" 0 2 "TestSomethingTimingSensitive" -- FIRST_RC=1 FIRST_OUT="$FLAKE" RETRY_RC=0
scenario "a package that fails twice fails the leg" 1 2 "failed again on retry" -- FIRST_RC=1 FIRST_OUT="$FLAKE" RETRY_RC=1
scenario "a build failure is deterministic: not retried" 1 1 "not retried" -- FIRST_RC=1 FIRST_OUT=$'FAIL\tmycorrhizal/x [build failed]'
scenario "a failure with no identifiable package is not retried" 1 1 "no identifiable failed package" -- FIRST_RC=1 FIRST_OUT="boom"
scenario "more than the cap of failed packages is a regression: not retried" 1 1 "a regression, not a flake" -- FIRST_RC=1 FIRST_OUT=$'FAIL\tmycorrhizal/a\t1.0s\nFAIL\tmycorrhizal/b\t1.0s\nFAIL\tmycorrhizal/c\t1.0s\nFAIL\tmycorrhizal/d\t1.0s'
scenario "the cap is configurable" 0 2 "FLAKE:" -- FIRST_RC=1 FIRST_OUT=$'FAIL\tmycorrhizal/a\t1.0s\nFAIL\tmycorrhizal/b\t1.0s' RETRY_RC=0 GO_TEST_RETRY_MAX_PKGS=2
scenario "a timeout (no --- FAIL line) is still retried and says so" 0 2 "none recorded" -- FIRST_RC=1 FIRST_OUT=$'panic: test timed out after 10m0s\nFAIL\tmycorrhizal/internal/schemafixture\t600.012s' RETRY_RC=0

echo "$pass passed, $fail failed"
[ "$fail" -eq 0 ]
