#!/usr/bin/env bash
#
# Tests for ../release-obligations.sh (issue #1195).
#
# The bug this pins: the two final-release-only obligations (the ASVS §10
# changelog-row gate and the per-release adversarial-delta gate) were inline in
# release.yml behind `if: is_rc != true` and never ran when promote-rc.yml
# turned an RC into the final. The shared script is now the one definition both
# call; these cases pin its decisions and exit codes.
#
# `git` and `go` are stubbed on PATH so the decisions are exercised without a
# real repository or Go toolchain (the same technique run-fuzz-targets.test.sh
# uses to stub `go`).

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$SCRIPT_DIR/../release-obligations.sh"

pass=0
fail=0

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
WORK="$tmp/work"
STUBS="$tmp/bin"
mkdir -p "$WORK/backend" "$STUBS"

# --- stubs -----------------------------------------------------------------

cat > "$STUBS/git" <<'GIT'
#!/usr/bin/env bash
case "${1:-}" in
  describe)
    if [ -n "${STUB_PREV_TAG:-}" ]; then printf '%s\n' "$STUB_PREV_TAG"; exit 0; fi
    exit 1
    ;;
  diff)
    if [ "${STUB_ASVS_ROW:-0}" = "1" ]; then
      printf '+| 2.10 | 2026-09-21 | (see PR) | ASVS L2 with 23 documented exceptions | a change |\n'
    fi
    # Present regardless of the call form; only the `^\+\|N.N` row is counted.
    printf 'backend/routes/stub_test.go\n'
    exit 0
    ;;
esac
exit 0
GIT

cat > "$STUBS/go" <<'GO'
#!/usr/bin/env bash
# Reconstructs cmd/adversarialdelta's contract: a non-empty -ack is an
# acceptance (exit 0); otherwise it fails, as an unrecorded delta would.
ack=""
prev=""
for a in "$@"; do
  if [ "$prev" = "-ack" ]; then ack="$a"; fi
  prev="$a"
done
if [ -n "$ack" ]; then
  echo "adversarial delta: acknowledged -- $ack"
  exit 0
fi
echo "::error::release stub: vX.Y.Z added security-relevant surface (route); ledger has no row."
exit "${STUB_ADV_EXIT:-1}"
GO

chmod +x "$STUBS/git" "$STUBS/go"
export PATH="$STUBS:$PATH"

# --- harness ---------------------------------------------------------------

# assert_case <name> <want-status> <want-substring> [args...]
assert_case() {
  local name="$1" want_status="$2" want_sub="$3"
  shift 3
  local out status
  out="$(cd "$WORK" && bash "$SCRIPT" "$@" 2>&1)"
  status=$?
  if [ "$status" -eq "$want_status" ] && printf '%s' "$out" | grep -qF -- "$want_sub"; then
    pass=$((pass + 1))
    echo "PASS: $name"
  else
    fail=$((fail + 1))
    echo "FAIL: $name (exit=$status want=$want_status; want substring: '$want_sub')"
    printf '%s\n' "$out" | sed 's/^/    /'
  fi
}

# --- asvs ------------------------------------------------------------------

unset STUB_PREV_TAG
assert_case "asvs: no previous release tag -> baseline pass" 0 "baseline" asvs

export STUB_PREV_TAG="v0.9.0"
export STUB_ASVS_ROW=1
assert_case "asvs: a new section-10 row -> pass" 0 "new changelog row" asvs

export STUB_ASVS_ROW=0
assert_case "asvs: no new row, no ack -> fail" 1 "no new section-10 changelog row" asvs

assert_case "asvs: no new row, recorded ack -> pass" 0 "acknowledged current" asvs --ack "no code changed since the last re-verification"

assert_case "asvs: unknown option -> usage error" 2 "unknown asvs option" asvs --bogus

# --- adversarial -----------------------------------------------------------

unset STUB_PREV_TAG
assert_case "adversarial: no previous release tag -> baseline pass" 0 "baseline" adversarial --release v1.0.0

export STUB_PREV_TAG="v0.9.0"
export STUB_ADV_EXIT=1
assert_case "adversarial: a touched class with no ledger row -> fail" 1 "Adversarial-delta gate failed" adversarial --release v1.0.0

assert_case "adversarial: recorded ack passes through -> pass" 0 "acknowledged" adversarial --release v1.0.0 --ack "test-only route change"

assert_case "adversarial: would-block downgraded under --dry-run -> pass" 0 "would block a real release" adversarial --release v1.0.0 --dry-run

assert_case "adversarial: missing --release -> usage error" 2 "requires --release" adversarial

assert_case "unknown command -> usage error" 2 "unknown command" frobnicate

# --- summary ---------------------------------------------------------------

echo
echo "release-obligations.test.sh: $pass passed, $fail failed"
[ "$fail" -eq 0 ]
