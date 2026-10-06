#!/usr/bin/env bash
# Issue #1487: run the ZAP automation plan, then the zapgate verdict, retrying
# the SCAN once -- and only for the one outcome that is a scanner flake rather
# than a verdict on the app.
#
# Release failure data (issue #1487): two release cuts were lost to zapgate's
# canary self-test ("no High/Medium alert for plugin 40012 found on the canary
# -- the scan is blind"): ZAP's JVM drops in-progress alert data under memory
# pressure (see issue #1278), so a clean app can produce a report that cannot
# prove the scan saw anything. zapgate exits `3` for EXACTLY that case (the
# self-test is the sole failure). Any other non-zero exit -- an unaccepted
# High/Medium app finding, an unreadable report -- is a real verdict and is
# NEVER retried: re-rolling a scan until it stops reporting a finding would be
# masking a defect.
#
# Env:
#   ZAP_MAX_ATTEMPTS   total scan attempts, default 2 (one retry)
#   ZAP_IMAGE          the pinned zaproxy image reference (required)
#   ZAP_AUTH_HEADER, ZAP_AUTH_HEADER_VALUE, ZAP_AUTH_HEADER_SITE  passed to ZAP
# Run from the repository root.
set -euo pipefail

max_attempts="${ZAP_MAX_ATTEMPTS:-2}"
image="${ZAP_IMAGE:?ZAP_IMAGE must name the pinned zaproxy image}"
exit_blind=3 # keep in sync with backend/cmd/zapgate's exitBlind

# The zaproxy image runs as uid 1000 (`zap`); the checkout is owned by the
# runner user, so the container could read the plan out of the bind mount but
# not create report.json in it. ZAP reported that only at the end, after a full
# active scan.
chmod a+rwx zap

run_scan() {
  rm -f zap/report.json
  local rc=0
  docker run --rm --network host \
    -e ZAP_AUTH_HEADER \
    -e ZAP_AUTH_HEADER_VALUE \
    -e ZAP_AUTH_HEADER_SITE \
    -v "$PWD/zap:/zap/wrk:rw" \
    -v "$PWD/backend/openapi.yaml:/zap/openapi.yaml:ro" \
    "$image" \
    zap.sh -Xmx6g -cmd -autorun /zap/wrk/zap-dast.yaml || rc=$?

  # zap.sh -autorun exit codes: 0 clean, 1 the plan had *failures*, 2 the plan
  # had *warnings* and no failure. A warning must not fail the build here --
  # zapgate is the gate, and letting an unrelated plan warning preempt it means
  # the security verdict never gets computed.
  if [ "$rc" -ne 0 ] && [ "$rc" -ne 2 ]; then
    echo "::error::ZAP exited $rc (automation plan failure)"
    return "$rc"
  fi

  # An exit code is not evidence the report exists: ZAP reports a failed
  # report-generation job at the very end. Fail here, where the cause is
  # obvious, rather than in the gate.
  if ! test -s zap/report.json; then
    echo "::error::ZAP produced no report.json -- gate cannot run"
    return 1
  fi
}

attempt=1
while :; do
  echo "::group::ZAP scan, attempt ${attempt} of ${max_attempts}"
  run_scan || exit $?
  echo "::endgroup::"

  gate_rc=0
  (cd backend && go run ./cmd/zapgate) || gate_rc=$?
  if [ "$gate_rc" -eq 0 ]; then
    if [ "$attempt" -gt 1 ]; then
      echo "::warning::ZAP scan was blind on attempt $((attempt - 1)) and clean on attempt ${attempt} (scanner flake, issue #1487) -- see the earlier attempt's log"
    fi
    exit 0
  fi
  if [ "$gate_rc" -ne "$exit_blind" ]; then
    echo "::error::zapgate rejected the scan (exit ${gate_rc}): a real finding, not retried"
    exit "$gate_rc"
  fi
  if [ "$attempt" -ge "$max_attempts" ]; then
    echo "::error::ZAP scan was blind on all ${max_attempts} attempts"
    exit "$gate_rc"
  fi
  echo "::warning::ZAP scan was blind on attempt ${attempt} (canary self-test missing) -- retrying once"
  attempt=$((attempt + 1))
done
