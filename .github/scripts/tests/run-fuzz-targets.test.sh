#!/usr/bin/env bash
#
# Tests for ../run-fuzz-targets.sh (issue #712).
#
# The golang/go#75804 deadline-boundary race is intermittent, so instead of
# trying to reproduce it live the hardening is exercised against a stubbed
# `go` binary that deterministically produces each outcome:
#   pass               -> clean run
#   deadline-once      -> the 1st invocation fails with "context deadline
#                         exceeded", every later one passes (the retry must
#                         trip and recover)
#   deadline-always    -> every invocation fails with "context deadline
#                         exceeded" (the retry trips, then soft-fails)
#   crash              -> "Failing input written to" (must fail hard)
#   crash-with-deadline-> both strings present (must fail hard — a crash is
#                         never softened just because the deadline string is
#                         also in the output)
#   other              -> a non-deadline failure, e.g. a seed replay of a
#                         previously-found crash (must fail hard)
#
# Asserts the script's exit code, the number of `go` invocations (retries are
# visible as extra invocations), and the expected markers in its output.

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$SCRIPT_DIR/../run-fuzz-targets.sh"

pass=0
fail=0

run_test() {
  local name="$1" outcome="$2" want_exit="$3" want_calls="$4"
  shift 4

  local workdir log calls status ok
  workdir="$(mktemp -d)"
  log="$(mktemp)"
  calls="$(mktemp)"

  cat >"$workdir/go" <<EOF
#!/usr/bin/env bash
echo "\$*" >> "\$FAKE_GO_LOG"
case "\$FAKE_GO_OUTCOME" in
  pass)
    echo "fuzz: elapsed: 1s, execs: 100 (100/sec), new interesting: 0 (total: 0)"
    exit 0
    ;;
  deadline-once)
    if [ ! -f "\$FAKE_GO_CALLS_FILE" ]; then
      touch "\$FAKE_GO_CALLS_FILE"
      echo "fuzz: elapsed: 1s, execs: 100 (100/sec), new interesting: 0 (total: 0)"
      echo "--- FAIL: FuzzX (1.00s)"
      echo "    context deadline exceeded"
      exit 1
    fi
    echo "fuzz: elapsed: 1s, execs: 100 (100/sec), new interesting: 0 (total: 0)"
    exit 0
    ;;
  deadline-always)
    echo "fuzz: elapsed: 1s, execs: 100 (100/sec), new interesting: 0 (total: 0)"
    echo "--- FAIL: FuzzX (1.00s)"
    echo "    context deadline exceeded"
    exit 1
    ;;
  crash)
    echo "fuzz: elapsed: 1s, execs: 100 (100/sec), new interesting: 0 (total: 0)"
    echo "fuzz: minimizing..."
    echo "Failing input written to testdata/fuzz/FuzzX/abc123"
    echo "To re-run: go test -run=FuzzX/abc123"
    exit 1
    ;;
  crash-with-deadline)
    echo "fuzz: elapsed: 1s, execs: 100 (100/sec), new interesting: 0 (total: 0)"
    echo "--- FAIL: FuzzX (1.00s)"
    echo "    context deadline exceeded"
    echo "Failing input written to testdata/fuzz/FuzzX/abc123"
    exit 1
    ;;
  other)
    echo "failure while testing seed corpus entry: FuzzX/abc123"
    echo "testing.go:1927: panic: boom"
    exit 1
    ;;
esac
EOF
  chmod +x "$workdir/go"

  FAKE_GO_OUTCOME="$outcome" \
  FAKE_GO_LOG="$calls" \
  FAKE_GO_CALLS_FILE="$workdir/calls.marker" \
  PATH="$workdir:$PATH" \
  FUZZTIME=1s \
    bash "$SCRIPT" >"$log" 2>&1
  status=$?

  ok=1
  [ "$status" -eq "$want_exit" ] || ok=0
  invocation_count="$(wc -l <"$calls" | tr -d ' ')"
  [ "$invocation_count" -eq "$want_calls" ] || ok=0
  for needle in "$@"; do
    case "$needle" in
      '!'*)
        if grep -q "${needle#!}" "$log"; then
          ok=0
        fi
        ;;
      *)
        if ! grep -q "$needle" "$log"; then
          ok=0
        fi
        ;;
    esac
  done

  if [ "$ok" -eq 1 ]; then
    pass=$((pass + 1))
    echo "PASS: $name"
  else
    fail=$((fail + 1))
    echo "FAIL: $name (exit=$status want=$want_exit, invocations=$invocation_count want=$want_calls)"
    sed 's/^/    | /' "$log"
  fi
  rm -rf "$workdir" "$log" "$calls"
}

run_test "clean pass runs all targets once with the fuzztime budget" \
  pass 0 8 \
  "fuzztime=1s"

run_test "deadline race on the first target is retried and recovers" \
  deadline-once 0 9 \
  "retrying once" \
  "!soft failure"

run_test "deadline race that repeats is a soft failure (nightly stays green)" \
  deadline-always 0 16 \
  "soft failure" \
  "!found a crash"

run_test "real crash fails hard and is never retried or softened" \
  crash 1 8 \
  "found a crash" \
  "one or more fuzz targets failed" \
  "!retrying once" \
  "!soft failure"

run_test "crash is still hard even when the deadline string is also present" \
  crash-with-deadline 1 8 \
  "found a crash" \
  "!soft failure"

run_test "seed-replay of a previously-found crash fails hard (no deadline string)" \
  other 1 8 \
  "failed (exit 1)" \
  "one or more fuzz targets failed" \
  "!retrying once" \
  "!soft failure"

echo ""
echo "$pass passed, $fail failed"
if [ "$fail" -ne 0 ]; then
  exit 1
fi
exit 0
