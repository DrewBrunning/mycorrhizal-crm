#!/usr/bin/env bash
#
# Tests for ../deepseek-offpeak.sh.
#
# DeepSeek bills 01:00-04:00 and 06:00-10:00 UTC Monday-Friday at the peak
# rate; every other time (including all weekend) is off-peak at half price.
# An agent-eval dispatch must refuse to start or run into a peak window, or
# accept peak billing with --allow-peak.

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$SCRIPT_DIR/../deepseek-offpeak.sh"

pass=0
fail=0

run_case() { # $1 = name, $2 = UTC now, $3 = expected exit, rest = flags
  local name="$1" now="$2" want="$3"
  shift 3
  local out code
  out="$(DEEPSEEK_OFFPEAK_NOW="$now" bash "$SCRIPT" "$@" 2>&1)"
  code=$?
  if [ "$code" -eq "$want" ]; then
    pass=$((pass + 1))
    echo "PASS: $name"
  else
    fail=$((fail + 1))
    echo "FAIL: $name (want exit $want, got $code; out=$out)"
  fi
}

# 2026-11-02 is a Monday; 2026-11-06 Friday; 2026-11-07 Saturday; 2026-11-08 Sunday.
run_case "Monday 00:59 is off-peak" 2026-11-02T00:59:00Z 0
run_case "Monday 01:00 is peak" 2026-11-02T01:00:00Z 1
run_case "Monday 03:59 is peak" 2026-11-02T03:59:00Z 1
run_case "Monday 04:00 is off-peak" 2026-11-02T04:00:00Z 0
run_case "Monday 05:00 is off-peak" 2026-11-02T05:00:00Z 0
run_case "Monday 05:59 is off-peak at the start" 2026-11-02T05:59:00Z 0
run_case "Monday 06:00 is peak" 2026-11-02T06:00:00Z 1
run_case "Monday 09:59 is peak" 2026-11-02T09:59:00Z 1
run_case "Monday 10:00 is off-peak" 2026-11-02T10:00:00Z 0
run_case "Friday 09:30 is peak" 2026-11-06T09:30:00Z 1
run_case "Saturday 02:00 is off-peak" 2026-11-07T02:00:00Z 0
run_case "Sunday 07:00 is off-peak" 2026-11-08T07:00:00Z 0

# A run starting off-peak can still cross into a peak window; the margin is
# the workflow timeout, so the guard rejects a start that could not finish
# before the next peak window.
run_case "Monday 04:00 with a 90m run stays off-peak" 2026-11-02T04:00:00Z 0 --margin-minutes 90
run_case "Monday 04:30 with a 90m run reaches 06:00 peak" 2026-11-02T04:30:00Z 1 --margin-minutes 90
run_case "Monday 05:00 with a 90m run reaches 06:30 peak" 2026-11-02T05:00:00Z 1 --margin-minutes 90
run_case "Sunday 23:00 with a 90m run ends before Monday peak" 2026-11-08T23:00:00Z 0 --margin-minutes 90
run_case "Sunday 23:30 with a 90m run reaches Monday 01:00 peak" 2026-11-08T23:30:00Z 1 --margin-minutes 90

run_case "peak with --allow-peak is accepted" 2026-11-02T02:00:00Z 0 --allow-peak

# Usage errors are distinct from a peak refusal (exit 2, not 1).
run_case "unknown flag is a usage error" 2026-11-02T02:00:00Z 2 --bogus

echo ""
echo "$pass passed, $fail failed"
[ "$fail" -eq 0 ]
