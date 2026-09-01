#!/usr/bin/env bash
# DEPLOY-01 (issue #450) misconfiguration case. Rebuilds a fresh .env from the
# documented template with a single broken value, brings the shipped
# docker-compose stack up, and asserts:
#   1. the app never becomes usable (/health/live never answers), and
#   2. the container log names the offending environment variable.
#
# Usage: deploy-smoke-misconfig.sh <env-key> <replacement-line> <log-needle>
#   env-key          key whose value is being broken (e.g. JWT_SECRET_KEY)
#   replacement-line  the full KEY=VALUE line to write into .env
#   log-needle        substring the startup failure must contain
#
# env_file entries override the image's Dockerfile ENV defaults, so appending
# a line here is enough to override e.g. ATTACHMENTS_DIR.
set -euo pipefail

env_key="$1"
replacement="$2"
needle="$3"

cleanup() {
  docker compose down -v >/dev/null 2>&1 || true
}
trap cleanup EXIT

# Fresh, otherwise-valid .env — only the injected value is wrong.
cp .env.example .env
secret="$(openssl rand -base64 32)"
sed -i "s|^JWT_SECRET_KEY=.*|JWT_SECRET_KEY=${secret}|" .env

if grep -q "^${env_key}=" .env; then
  sed -i "s|^${env_key}=.*|${replacement}|" .env
else
  printf '%s\n' "${replacement}" >> .env
fi
echo "Broke ${env_key}: $(grep -E "^${env_key}=" .env || echo '<removed/blank>')"

# `restart: unless-stopped` makes a config-panic container crash-loop, so do
# not gate on `--wait`; bring it up and poll instead. The backend process is
# supervised, so a panic loop ends with supervisord giving up rather than the
# container exiting — break as soon as that happens, else wait the full
# window in case startup is merely slow.
docker compose up -d --build

deadline=$(( SECONDS + 90 ))
while (( SECONDS < deadline )); do
  if curl -fsS -o /dev/null http://localhost:7300/health/live 2>/dev/null; then
    echo "::error::instance became usable despite a broken ${env_key} — the misconfiguration was not rejected"
    docker compose logs --no-color mycorrhizal || true
    exit 1
  fi
  if docker compose logs --no-color mycorrhizal 2>&1 | grep -q 'backend entered FATAL state'; then
    echo "backend gave up restarting — startup rejected as expected."
    break
  fi
  sleep 3
done

logs="$(docker compose logs --no-color mycorrhizal 2>&1 || true)"
echo "----- startup log for broken ${env_key} -----"
echo "${logs}"
echo "---------------------------------------------"

if ! grep -qF "${needle}" <<<"${logs}"; then
  echo "::error::startup failure for broken ${env_key} did not mention '${needle}'"
  exit 1
fi
echo "OK: broken ${env_key} failed startup and the log named '${needle}'."
