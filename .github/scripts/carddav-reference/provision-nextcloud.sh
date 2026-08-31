#!/usr/bin/env bash
#
# CI provisioning for Nextcloud (nextcloud:stable image) for the TEST-09
# reference-server matrix (.github/workflows/carddav-e2e.yml).
#
# Nextcloud has a proper headless CLI (occ), so provisioning is: install,
# create the user, create the address book, then wait for the DAV endpoint.
#
# Usage: provision-nextcloud.sh [container-name] [port]
set -euo pipefail

NAME="${1:-nextcloud}"
PORT="${2:-5234}"
USER="syncuser"
# Nextcloud's default password policy rejects "syncsecret" (it is in the
# compromised-password list), so use a compliant one — the workflow passes it
# to the test as MYCORRHIZAL_CARDDAV_PASSWORD.
PASSWORD="SynCsecret-8f9a"

docker rm -f "$NAME" >/dev/null 2>&1 || true
docker run -d --name "$NAME" -p "$PORT:80" -e SQLITE_DATABASE=nextcloud nextcloud:stable >/dev/null

# Wait for the web server, then run the first-time install.
for _ in $(seq 1 120); do
  code=$(curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:$PORT/" 2>/dev/null || true)
  if [ "$code" = "200" ] || [ "$code" = "303" ] || [ "$code" = "302" ]; then
    break
  fi
  sleep 1
done

docker exec -u www-data "$NAME" php occ maintenance:install \
  --database sqlite --database-name nextcloud \
  --admin-user admin --admin-pass adminpass123 >/dev/null

# The password-from-env flow needs OC_PASS and a >=10 char password.
docker exec -e OC_PASS="$PASSWORD" -u www-data "$NAME" php occ user:add \
  --password-from-env --display-name "Sync User" "$USER" >/dev/null

docker exec -u www-data "$NAME" php occ dav:create-addressbook "$USER" contacts >/dev/null

# Readiness: the address book answers an authenticated PROPFIND.
for _ in $(seq 1 120); do
  code=$(curl -s -o /dev/null -w "%{http_code}" -u "$USER:$PASSWORD" \
    -X PROPFIND -H 'Depth: 0' "http://127.0.0.1:$PORT/remote.php/dav/addressbooks/users/$USER/contacts/")
  if [ "$code" = "207" ]; then
    echo "Nextcloud ready"
    exit 0
  fi
  sleep 1
done
echo "Nextcloud failed to become ready" >&2
docker logs "$NAME" 2>&1 | tail -20
exit 1
