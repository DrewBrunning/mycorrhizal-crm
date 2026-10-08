#!/usr/bin/env bash
#
# Install Playwright browsers (and, via --with-deps, their apt packages) with a
# bounded per-attempt timeout and retry (issue #1549). A stalled CDN or apt
# mirror used to hang the step for ~20 minutes; now each attempt is bounded and
# retried.
#
# Between attempts, any apt-get/dpkg left behind by a timed-out attempt is
# killed and the dpkg lock waited out: `timeout` signals its own process group,
# but `sudo apt-get` (started by `playwright install --with-deps`) can run in
# its own session and outlive it, holding /var/lib/dpkg/lock-frontend so every
# retry would fail on the lock.
#
# Usage (from frontend/): bash ../.github/scripts/playwright-install-retry.sh <browser>...
#
# The budget lines below are read by backend/internal/e2einstall's test, which
# requires each install step's timeout-minutes to cover the worst case:
#   attempts*(per_attempt+kill_after) + (attempts-1)*(backoff+lock_wait)
set -uo pipefail

attempts=3
per_attempt=240
kill_after=15
backoff=15
lock_wait=60

if [ "$#" -eq 0 ]; then
  echo "usage: playwright-install-retry.sh <browser>..." >&2
  exit 2
fi

release_apt() {
  sudo pkill -9 -x apt-get >/dev/null 2>&1 || true
  sudo pkill -9 -x dpkg >/dev/null 2>&1 || true
  local waited=0
  while sudo fuser /var/lib/dpkg/lock-frontend /var/lib/dpkg/lock >/dev/null 2>&1; do
    if [ "$waited" -ge "$lock_wait" ]; then
      echo "::warning::dpkg lock still held after ${lock_wait}s" >&2
      break
    fi
    sleep 2
    waited=$((waited + 2))
  done
  # Finish any install an interrupted dpkg left half-configured.
  sudo dpkg --configure -a >/dev/null 2>&1 || true
}

for attempt in $(seq 1 "$attempts"); do
  if timeout --kill-after="$kill_after" "$per_attempt" npx playwright install --with-deps "$@"; then
    exit 0
  fi
  echo "::warning::playwright install ($*) attempt $attempt/$attempts failed or timed out"
  if [ "$attempt" -lt "$attempts" ]; then
    release_apt
    sleep "$backoff"
  fi
done
exit 1
