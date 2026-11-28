#!/usr/bin/env bash
# Regenerate docs/assets/screenshots/*.png from the demo stack (issue #2244).
#
# Starts a fresh pen-test/demo stack (docker-compose.pentest.yml, `demo`
# profile), reads its seeded login, and runs the Playwright generator in
# frontend/e2e/docs-screenshots. Needs Docker, jq, Node >= 24.15 and a yarn
# install in frontend/. Nothing here is part of CI; the PNGs are committed.
#
# The compose file pins container_name to mycorrhizal-pentest, so a pen-test
# stack that is already running must be stopped first; the script refuses to
# start rather than fail half-way through `up`.
#
#   scripts/docs-screenshots.sh            # port 7391, project name docsdemo
#   DOCS_KEEP_STACK=1 scripts/...          # leave the stack up afterwards
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"
port="${DOCS_PORT:-7391}"
project=docsdemo
export PENTEST_PORT="$port" PENTEST_BASE_URL="http://localhost:$port" PENTEST_PROFILE=demo
compose=(docker compose -p "$project" -f docker-compose.pentest.yml)

cleanup() {
  if [ -z "${DOCS_KEEP_STACK:-}" ]; then "${compose[@]}" down -v >/dev/null 2>&1 || true; fi
}

command -v jq >/dev/null || { echo "docs-screenshots: jq is required" >&2; exit 1; }
if docker ps -a --format '{{.Names}}' | grep -qx mycorrhizal-pentest &&
  ! "${compose[@]}" ps -a --format '{{.Name}}' | grep -qx mycorrhizal-pentest; then
  echo "docs-screenshots: a mycorrhizal-pentest container from another compose project exists;" >&2
  echo "  stop it first (docker compose -f docker-compose.pentest.yml down)" >&2
  exit 1
fi

trap cleanup EXIT

"${compose[@]}" up -d --build --wait
creds="$("${compose[@]}" exec -T mycorrhizal cat /app/data/credentials.json)"
DOCS_USERNAME="$(jq -r '.users[0].username' <<<"$creds")"
DOCS_PASSWORD="$(jq -r '.users[0].password' <<<"$creds")"
export DOCS_USERNAME DOCS_PASSWORD DOCS_BASE_URL="http://localhost:$port"

mkdir -p docs/assets/screenshots
(cd frontend && npx playwright test -c playwright.docs.config.ts)
echo "Screenshots written to docs/assets/screenshots/"
