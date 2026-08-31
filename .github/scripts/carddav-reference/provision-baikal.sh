#!/usr/bin/env bash
#
# CI provisioning for Baikal (ckulka/baikal image) for the TEST-09
# reference-server matrix (.github/workflows/carddav-e2e.yml).
#
# Baikal has no headless setup story: the web installer wizard writes
# config/ + system.sqlite, and the admin SPA is not scriptable. So this script
# (1) drives the two-step install wizard over HTTP with stdlib-only Python,
# (2) normalizes the dav_auth_type YAML value (the wizard lowercases "Basic"
# but Baikal's auth-type check is case-sensitive: `$this->authType === 'Basic'`),
# and (3) seeds the DAV user + principal + address book directly in the SQLite
# DB (the SabreDAV PDO schema).
#
# Usage: provision-baikal.sh [container-name] [port] [data-dir]
set -euo pipefail

NAME="${1:-baikal}"
PORT="${2:-5233}"
DATA_DIR="${3:-/tmp/baikal-data}"
REALM="BaikalDAV"
USER="syncuser"
PASSWORD="syncsecret"
DB="/var/www/baikal/Specific/db/db.sqlite"

# A stale data dir (with a www-data-owned .htaccess) blocks the wizard; wipe it.
rm -rf "$DATA_DIR" 2>/dev/null || sudo rm -rf "$DATA_DIR" 2>/dev/null || true
docker rm -f "$NAME" >/dev/null 2>&1 || true
docker run -d --name "$NAME" -p "$PORT:80" -e BAIKAL_DB=sqlite \
  -v "$DATA_DIR:/var/www/baikal/config" ckulka/baikal:latest >/dev/null

# Apache needs a few seconds; wait before driving the wizard.
for _ in $(seq 1 60); do
  code=$(curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:$PORT/admin/install/" 2>/dev/null || true)
  if [ "$code" = "200" ]; then
    break
  fi
  sleep 1
done

python3 - "$PORT" "$REALM" <<'PYEOF'
import hashlib
import re
import sys
import urllib.parse
import urllib.request
from http.cookiejar import CookieJar

base = f"http://127.0.0.1:{sys.argv[1]}"
realm = sys.argv[2]
admin_pass = "admin-secret"
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(CookieJar()))


def get(path):
    return opener.open(base + path).read().decode("utf-8")


def post(path, data):
    req = urllib.request.Request(base + path, data=urllib.parse.urlencode(data).encode())
    req.add_header("Content-Type", "application/x-www-form-urlencoded")
    return opener.open(req)


def csrf(html):
    m = re.search(r'name="CSRF_TOKEN" value="([0-9a-f]+)"', html)
    return m.group(1) if m else ""


page = get("/admin/install/")
if 'name="CSRF_TOKEN"' not in page:
    print("already installed", flush=True)
    sys.exit(0)

admin_hash = hashlib.sha256(f"admin:{realm}:{admin_pass}".encode()).hexdigest()
token = csrf(page)
post("/admin/install/", {
    "CSRF_TOKEN": token,
    "Baikal_Model_Config_Standard::submitted": "1",
    "refreshed": "0",
    "data[admin_passwordhash]": admin_hash,
    "data[admin_passwordhash_confirm]": admin_hash,
    "data[cal_enabled]": "1",
    "data[card_enabled]": "1",
    "data[dav_auth_type]": "Basic",
    "data[invite_from]": "noreply@localhost",
    "data[timezone]": "Europe/Berlin",
    "witness[admin_passwordhash]": "1",
    "witness[admin_passwordhash_confirm]": "1",
    "witness[cal_enabled]": "1",
    "witness[card_enabled]": "1",
    "witness[dav_auth_type]": "1",
    "witness[invite_from]": "1",
    "witness[timezone]": "1",
})

page = get("/admin/install/")
post("/admin/install/", {
    "CSRF_TOKEN": csrf(page),
    "Baikal_Model_Config_Database::submitted": "1",
    "refreshed": "0",
    "data[backend]": "sqlite",
    "data[sqlite_file]": "/var/www/baikal/Specific/db/db.sqlite",
    "witness[backend]": "1",
    "witness[sqlite_file]": "1",
})
print("wizard done", flush=True)
PYEOF

# The wizard lowercases the listbox value ("Basic" -> "basic") and Baikal's
# auth-type check is case-sensitive; normalize the YAML to the capital form.
docker exec -u root "$NAME" sh -c \
  "sed -i 's/dav_auth_type: basic/dav_auth_type: Basic/' /var/www/baikal/config/baikal.yaml"

# Seed the DAV user (digesta1 = md5(user:realm:password), the PDOBasicAuth
# check), the principal, and the address book.
DIGEST=$(python3 -c "import hashlib,sys;print(hashlib.md5((sys.argv[1]+':'+sys.argv[2]+':'+sys.argv[3]).encode()).hexdigest())" "$USER" "$REALM" "$PASSWORD")
docker exec -i "$NAME" php -r '
$pdo = new PDO("sqlite:" . $argv[1]);
$u = $argv[2]; $a1 = $argv[3];
$pdo->exec("INSERT INTO users (username, digesta1) VALUES (\"" . $u . "\", \"" . $a1 . "\")");
$pdo->exec("INSERT INTO principals (uri, email, displayname) VALUES (\"principals/" . $u . "\", \"" . $u . "@example.com\", \"Sync User\")");
$pdo->exec("INSERT INTO addressbooks (principaluri, displayname, uri, synctoken) VALUES (\"principals/" . $u . "\", \"Contacts\", \"contacts\", 1)");
echo "seeded\n";
' -- "$DB" "$USER" "$DIGEST"

# Readiness: the address book answers an authenticated PROPFIND.
for _ in $(seq 1 60); do
  code=$(curl -s -o /dev/null -w "%{http_code}" -u "$USER:$PASSWORD" \
    -X PROPFIND -H 'Depth: 0' "http://127.0.0.1:$PORT/dav.php/addressbooks/$USER/contacts/")
  if [ "$code" = "207" ]; then
    echo "Baikal ready"
    exit 0
  fi
  sleep 1
done
echo "Baikal failed to become ready" >&2
docker logs "$NAME" 2>&1 | tail -20
exit 1
