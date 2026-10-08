#!/usr/bin/env bash
#
# Tests for ../zap-scan-gate.sh (issue #1487).
#
# What these pin: the ONE case the DAST gate may retry is a blind scan
# (zapgate exit 3). A real finding (exit 1), a ZAP plan failure, or a missing
# report must never be re-rolled -- that would mask a defect. `docker` and `go`
# are stubs on PATH: `docker` records each scan and writes (or not) a
# report.json; `go` replays a scripted sequence of zapgate exit codes.

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$SCRIPT_DIR/../zap-scan-gate.sh"

indent() { printf "    | %s\n" "${1//$'\n'/$'\n    | '}"; }

pass=0
fail=0

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/bin"

# docker stub: counts scans (not pulls) in $SCAN_COUNT_FILE; exits $ZAP_RC; writes
# zap/report.json unless NO_REPORT=1.
cat > "$work/bin/docker" <<'STUB'
#!/usr/bin/env bash
# `docker pull` (the retrying pre-pull, issue #1566) is not a scan.
if [ "$1" = "pull" ]; then echo "pull $*" >> "$SCAN_COUNT_FILE.pulls"; exit 0; fi
# Every scan must run with --pull never so no second, unretried pull happens.
case " $* " in *" --pull never "*) ;; *) echo "scan without --pull never: $*" >&2; exit 99 ;; esac
n=$(cat "$SCAN_COUNT_FILE" 2>/dev/null || echo 0)
echo $((n + 1)) > "$SCAN_COUNT_FILE"
if [ "${NO_REPORT:-0}" != "1" ]; then echo '{}' > zap/report.json; fi
exit "${ZAP_RC:-0}"
STUB
chmod +x "$work/bin/docker"

# go stub: each `go run ./cmd/zapgate` pops the next code from $GATE_CODES
# (space-separated); the last one repeats.
cat > "$work/bin/go" <<'STUB'
#!/usr/bin/env bash
codes=($GATE_CODES)
n=$(cat "$GATE_COUNT_FILE" 2>/dev/null || echo 0)
echo $((n + 1)) > "$GATE_COUNT_FILE"
i=$n
if [ "$i" -ge "${#codes[@]}" ]; then i=$((${#codes[@]} - 1)); fi
exit "${codes[$i]}"
STUB
chmod +x "$work/bin/go"

# scenario NAME WANT_RC WANT_SCANS WANT_SUBSTRING -- ENV=VAL...
scenario() {
  local name="$1" want_rc="$2" want_scans="$3" want_out="$4"
  shift 5
  rm -rf "${work:?}/repo"
  mkdir -p "$work/repo/zap" "$work/repo/backend"
  : > "$work/scans"
  : > "$work/gates"
  local out rc scans
  out="$(cd "$work/repo" && env PATH="$work/bin:$PATH" SCAN_COUNT_FILE="$work/scans" \
    GATE_COUNT_FILE="$work/gates" ZAP_IMAGE=zap@sha256:x "$@" bash "$SCRIPT" 2>&1)"
  rc=$?
  scans="$(cat "$work/scans")"
  scans="${scans:-0}"
  if [ "$rc" -eq "$want_rc" ] && [ "$scans" -eq "$want_scans" ] && grep -qF -- "$want_out" <<<"$out"; then
    pass=$((pass + 1))
    echo "PASS: $name"
  else
    fail=$((fail + 1))
    echo "FAIL: $name (rc=$rc want=$want_rc, scans=$scans want=$want_scans, want substring '$want_out')"
    indent "$out"
  fi
}

scenario "clean scan passes on the first attempt, no retry" 0 1 "" -- GATE_CODES="0"
scenario "blind first scan is retried once and a clean second scan passes" 0 2 "scanner flake" -- GATE_CODES="3 0"
scenario "blind twice fails after exactly two scans" 3 2 "blind on all 2 attempts" -- GATE_CODES="3 3"
scenario "a real finding (exit 1) is never retried" 1 1 "a real finding, not retried" -- GATE_CODES="1 0"
scenario "a real finding after a blind scan is not retried a third time" 1 2 "a real finding, not retried" -- GATE_CODES="3 1 0"
scenario "ZAP_MAX_ATTEMPTS=1 disables the retry" 3 1 "blind on all 1 attempts" -- GATE_CODES="3 0" ZAP_MAX_ATTEMPTS=1
scenario "ZAP_MAX_ATTEMPTS=3 allows a third attempt" 0 3 "scanner flake" -- GATE_CODES="3 3 0" ZAP_MAX_ATTEMPTS=3
scenario "a ZAP plan failure (exit 1) is not retried and the gate never runs" 1 1 "ZAP exited 1" -- GATE_CODES="0" ZAP_RC=1
scenario "ZAP plan warning (exit 2) still reaches the gate" 0 1 "" -- GATE_CODES="0" ZAP_RC=2
scenario "no report.json fails before the gate, not retried" 1 1 "ZAP produced no report.json" -- GATE_CODES="0" NO_REPORT=1

# ZAP_IMAGE is required: an unpinned scan must not run.
rm -rf "${work:?}/repo"
mkdir -p "$work/repo/zap" "$work/repo/backend"
out="$(cd "$work/repo" && env -u ZAP_IMAGE PATH="$work/bin:$PATH" SCAN_COUNT_FILE="$work/scans" GATE_COUNT_FILE="$work/gates" GATE_CODES=0 bash "$SCRIPT" 2>&1)"
rc=$?
if [ "$rc" -ne 0 ] && grep -qF "ZAP_IMAGE must name the pinned zaproxy image" <<<"$out"; then
  pass=$((pass + 1))
  echo "PASS: missing ZAP_IMAGE is refused"
else
  fail=$((fail + 1))
  echo "FAIL: missing ZAP_IMAGE is refused (rc=$rc)"
  indent "$out"
fi

echo "$pass passed, $fail failed"
[ "$fail" -eq 0 ]
