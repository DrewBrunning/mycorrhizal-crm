#!/usr/bin/env bash
# Issue #1489: one leg of the real-release upgrade proof.
#
#   scripts/realrelease-leg.sh v1.0.0 [workdir]
#
# Boots the PUBLISHED image for the tag, drives it through its public API
# (cmd/realrelease seed), stops it, upgrades the data it wrote with the
# CURRENT code (cmd/realrelease verify), then plays the supported rollback
# (restore the mandatory pre-migration backup, start the OLD image over it
# again, read back, compare). Needs docker and a Go toolchain; all data is
# synthetic. Exit 0 = clean.
set -euo pipefail

tag="${1:?usage: realrelease-leg.sh vX.Y.Z [workdir]}"
repo="${REALRELEASE_IMAGE:-ghcr.io/drewbrunning/mycorrhizal-crm}"
image="${repo}:${tag#v}"
root="$(cd "$(dirname "$0")/.." && pwd)"
work="${2:-$(mktemp -d)}"
mkdir -p "$work"
name="realrelease-${tag#v}-$$"
port="${REALRELEASE_PORT:-$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1])')}"

cleanup() { docker rm -f "$name" >/dev/null 2>&1 || true; }
trap cleanup EXIT

(cd "$root/backend" && go build -o "$work/realrelease" ./cmd/realrelease)
rr="$work/realrelease"
secret="$("$rr" print-secret)"

if ! "$(dirname "$0")/../.github/scripts/docker-pull-retry.sh" "$image" >/dev/null; then
  echo "::error::cannot pull $image after retries - the tag is registered in SupportedReleases but its image is not published (docker-publish.yml still running or failed?), or the registry rate-limited the pull (toomanyrequests)"
  exit 1
fi

# $1 = optional dir holding data/ photos/ attachments/ to seed the container with
start_old() {
  cleanup
  docker create --name "$name" -p "127.0.0.1:${port}:8080" \
    -e JWT_SECRET_KEY="$secret" -e FRONTEND_URL="http://localhost:${port}" \
    -e DISABLE_REGISTRATION=false -e API_RATE_LIMIT_INTERVAL_MS=1 -e API_RATE_LIMIT_BURST=100000 \
    -e DB_RESTORE_DRILL_ENABLED=false "$image" >/dev/null
  if [ -n "${1:-}" ]; then
    chmod -R a+rwX "$1"
    docker cp "$1/data/." "$name:/app/data/"
    docker cp "$1/photos/." "$name:/app/static/photos/"
    docker cp "$1/attachments/." "$name:/app/static/attachments/"
  fi
  docker start "$name" >/dev/null
  for _ in $(seq 1 60); do
    if curl -sf "http://127.0.0.1:${port}/health/live" >/dev/null 2>&1; then return 0; fi
    sleep 1
  done
  docker logs "$name" | tail -30
  echo "::error::$image did not become healthy"
  return 1
}

echo "== $tag: seed through the published image"
start_old
"$rr" seed --base-url "http://127.0.0.1:${port}" --out "$work/out"
docker stop -t 30 "$name" >/dev/null
rm -rf "$work/install" "$work/original"
mkdir -p "$work/install"
docker cp "$name:/app/data" "$work/install/data"
docker cp "$name:/app/static/photos" "$work/install/photos"
docker cp "$name:/app/static/attachments" "$work/install/attachments"
cleanup
cp -a "$work/install" "$work/original"

echo "== $tag: upgrade in place with the current code, then read back"
"$rr" verify --db "$work/install/data/mycorrhizal.db" \
  --photos "$work/install/photos" --attachments "$work/install/attachments" \
  --creds "$work/out/credentials.json" --pre "$work/out/pre-snapshot.json" \
  --out "$work/out/post-snapshot.json"

echo "== $tag: rollback - restore the pre-migration backup, run the OLD image again"
# An already-current release (no pending migration) takes no pre-migration
# backup by design (database.migrateFileWithPreBackup), so the rollback target
# is the byte-copy of the data taken before the upgrade.
backup="$(ls "$work"/install/data/pre-migration/*.db 2>/dev/null | head -1 || true)"
if [ -z "$backup" ]; then
  echo "no pre-migration backup (release already at the current schema); rolling back to the pre-upgrade copy"
  backup="$work/original/data/mycorrhizal.db"
fi
rm -rf "$work/rollback"
mkdir -p "$work/rollback/data"
cp "$backup" "$work/rollback/data/mycorrhizal.db"
cp -a "$work/original/photos" "$work/original/attachments" "$work/rollback/"
start_old "$work/rollback"
"$rr" capture --base-url "http://127.0.0.1:${port}" --creds "$work/out/credentials.json" --out "$work/out/rollback-snapshot.json"
"$rr" compare --pre "$work/out/pre-snapshot.json" --post "$work/out/rollback-snapshot.json"
echo "== $tag: OK"
