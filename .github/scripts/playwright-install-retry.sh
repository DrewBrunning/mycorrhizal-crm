#!/usr/bin/env bash
#
# Install Playwright browsers (and, via --with-deps, their apt packages) with a
# bounded per-attempt timeout and retry (issue #1549), and a cache of the apt
# .deb packages so a slow Ubuntu mirror is not re-downloaded on every run
# (issue #1646).
#
# Between attempts, any apt-get/dpkg left behind by a timed-out attempt is
# killed and the dpkg lock waited out: `timeout` signals its own process group,
# but `sudo apt-get` (started by `playwright install --with-deps`) can run in
# its own session and outlive it, holding /var/lib/dpkg/lock-frontend so every
# retry would fail on the lock.
#
# Usage (from frontend/): bash ../.github/scripts/playwright-install-retry.sh <browser>...
#
# The apt .deb cache directory is cached by the workflow alongside
# ~/.cache/ms-playwright (the "Cache Playwright browsers and apt packages"
# step); override its location with PLAYWRIGHT_APT_CACHE. Copying into apt's
# archives directory needs root, so the copies are run under sudo.
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

apt_cache="${PLAYWRIGHT_APT_CACHE:-$HOME/.cache/playwright-apt}"
apt_archives=/var/cache/apt/archives

if [ "$#" -eq 0 ]; then
  echo "usage: playwright-install-retry.sh <browser>..." >&2
  exit 2
fi

# configure_apt bounds each individual download (issue #1912): with apt's
# default 120 s per-connection timeout, one stalled package (libflite1, 13.5 MB,
# in the Ubuntu 26.04 WebKit dependency set) burned 140 s of a 240 s attempt
# before apt gave up on it and moved on. On a healthy mirror the same 84 MB of
# packages download in ~1 s, so the failure was a stalled connection, not
# volume. A short per-connection timeout plus retries makes apt abandon a bad
# socket quickly and try another instead of spending the whole attempt on it.
configure_apt() {
  sudo tee /etc/apt/apt.conf.d/99-playwright-retry >/dev/null <<'EOF'
// Installed by .github/scripts/playwright-install-retry.sh (issue #1912).
// Short per-transfer timeout + retries so one stalled mirror connection
// cannot consume a whole bounded install attempt.
Acquire::Retries "5";
Acquire::http::Timeout "20";
Acquire::https::Timeout "20";
Acquire::http::Pipeline-Depth "0";
EOF
}

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

# seed_apt_cache copies the cached .deb packages into apt's archives directory,
# so `apt-get install` (which `playwright install --with-deps` runs) finds them
# locally instead of fetching them from a slow mirror. No-op when the cache is
# empty — the first run, or the first after a Playwright/OS bump.
seed_apt_cache() {
  [ -d "$apt_cache" ] || return 0
  if compgen -G "$apt_cache/*.deb" >/dev/null; then
    sudo mkdir -p "$apt_archives"
    sudo cp -f "$apt_cache"/*.deb "$apt_archives"/ 2>/dev/null || true
  fi
}

# collect_apt_cache saves every .deb apt has in its archives directory — those
# it downloaded this run plus any seeded ones — so the next run can install
# without downloading. The files are chowned back to the runner user because
# they were copied out under sudo, so the cache action can read and replace
# them.
collect_apt_cache() {
  if compgen -G "$apt_archives/*.deb" >/dev/null; then
    mkdir -p "$apt_cache"
    sudo cp -f "$apt_archives"/*.deb "$apt_cache"/ 2>/dev/null || true
    sudo chown -R "$(id -u):$(id -g)" "$apt_cache" 2>/dev/null || true
  fi
}

configure_apt
seed_apt_cache

for attempt in $(seq 1 "$attempts"); do
  if timeout --kill-after="$kill_after" "$per_attempt" npx playwright install --with-deps "$@"; then
    collect_apt_cache
    exit 0
  fi
  echo "::warning::playwright install ($*) attempt $attempt/$attempts failed or timed out"
  if [ "$attempt" -lt "$attempts" ]; then
    release_apt
    sleep "$backoff"
  fi
done
# Even a failed run keeps whatever debs it did download, so the next run
# resumes from further along instead of starting the download over.
collect_apt_cache
exit 1
