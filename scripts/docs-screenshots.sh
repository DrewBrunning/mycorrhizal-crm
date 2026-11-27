#!/usr/bin/env bash
# Regenerate docs/assets/screenshots/*.png from the demo stack (issue #2244).
#
# Starts the pen-test/demo stack (docker-compose.pentest.yml, `demo` profile) if
# it is not already running, reads its seeded login, and runs the Playwright
# generator in frontend/e2e/docs-screenshots. Needs Docker, Node >= 24.15 and a yarn install
# in frontend/. Nothing here is part of CI; the PNGs are committed.
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
trap cleanup EXIT

"${compose[@]}" up -d --build --wait
creds="$("${compose[@]}" exec -T mycorrhizal cat /app/data/credentials.json)"
DOCS_USERNAME="$(jq -r '.users[0].username' <<<"$creds")"
DOCS_PASSWORD="$(jq -r '.users[0].password' <<<"$creds")"
export DOCS_USERNAME DOCS_PASSWORD DOCS_BASE_URL="http://localhost:$port"

mkdir -p docs/assets/screenshots
(cd frontend && npx playwright test -c playwright.docs.config.ts)
echo "Screenshots written to docs/assets/screenshots/"
