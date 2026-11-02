#!/usr/bin/env bash
#
# Refuse an agent-eval run that would start or run inside a DeepSeek peak
# billing window.
#
# DeepSeek bills 01:00-04:00 and 06:00-10:00 UTC, Monday-Friday, at the peak
# rate. All weekend (and every other hour) is off-peak at half price. The
# scheduled collector runs on Sunday, but GitHub can delay a scheduled run by
# hours, so the window is checked at run time as well as in the cron.
#
# Usage:
#   deepseek-offpeak.sh [--allow-peak] [--margin-minutes N]
#
# --margin-minutes is the worst-case run duration; the guard rejects a start
# that could run into the next peak window. The default is 0 (start-time only).
# --allow-peak accepts peak billing deliberately and only warns.
#
# Exit 0: the run is off-peak (or peak was explicitly allowed).
# Exit 1: the run would touch a peak window.
# Exit 2: usage error.
#
# DEEPSEEK_OFFPEAK_NOW is a test seam (an ISO-8601 UTC timestamp); never set it
# in a workflow.
set -euo pipefail

usage() {
  echo "usage: $0 [--allow-peak] [--margin-minutes N]" >&2
}

allow_peak=false
margin=0
while [ "$#" -gt 0 ]; do
  case "$1" in
    --allow-peak)
      allow_peak=true
      shift
      ;;
    --margin-minutes)
      [ "$#" -ge 2 ] || {
        usage
        exit 2
      }
      margin="$2"
      case "$margin" in
        '' | *[!0-9]*)
          usage
          exit 2
          ;;
      esac
      shift 2
      ;;
    *)
      usage
      exit 2
      ;;
  esac
done

now="${DEEPSEEK_OFFPEAK_NOW:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
start="$(date -u -d "$now" +%s)"
end="$(date -u -d "$now + ${margin} minutes" +%s)"

# is_peak: Monday-Friday and an hour in [01:00,04:00) or [06:00,10:00).
is_peak() {
  local epoch="$1" day hour
  day="$(date -u -d "@$epoch" +%u)"
  hour="$(date -u -d "@$epoch" +%H)"
  hour=$((10#$hour))
  [ "$day" -ge 1 ] && [ "$day" -le 5 ] || return 1
  { [ "$hour" -ge 1 ] && [ "$hour" -lt 4 ]; } || { [ "$hour" -ge 6 ] && [ "$hour" -lt 10 ]; }
}

touches_peak=false
check() {
  if is_peak "$1"; then
    touches_peak=true
  fi
}

check "$start"
# Sample every UTC hour boundary inside the interval, plus the end, so a run
# that starts off-peak but crosses into a peak window is still refused.
epoch=$(((start / 3600 + 1) * 3600))
while [ "$epoch" -lt "$end" ]; do
  check "$epoch"
  epoch=$((epoch + 3600))
done
check "$end"

if [ "$touches_peak" = true ]; then
  if [ "$allow_peak" = true ]; then
    echo "::warning::--allow-peak accepted possible DeepSeek peak-rate billing for this run."
    exit 0
  fi
  echo "::error::Refusing an agent-eval run that would start or run into DeepSeek peak billing hours (01:00-04:00 and 06:00-10:00 UTC, Mon-Fri). Run off-peak, or pass --allow-peak to accept possible peak-rate billing." >&2
  exit 1
fi

echo "DeepSeek off-peak window confirmed."
