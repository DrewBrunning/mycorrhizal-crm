#!/bin/sh
set -e

# Remap the backend user to the host-provided PUID/PGID so files written to the
# mounted volumes (/app/data, /app/static/photos) get sane ownership, then hand
# off to supervisord (which keeps running as root and drops the backend process
# to appuser via `user=appuser`).

PUID="${PUID:-1001}"
PGID="${PGID:-1001}"

NEEDS_CHOWN=0

if [ "$(id -g appuser)" != "$PGID" ]; then
    groupmod -o -g "$PGID" appgroup
    NEEDS_CHOWN=1
fi

if [ "$(id -u appuser)" != "$PUID" ]; then
    usermod -o -u "$PUID" appuser
    NEEDS_CHOWN=1
fi

DATA_DIR="$(dirname "$SQLITE_DB_PATH")"

# On first startup the mounted directories may be owned by root
if [ "$(stat -c '%u:%g' "$DATA_DIR")" != "$PUID:$PGID" ] || \
    [ "$(stat -c '%u:%g' "$PROFILE_PHOTO_DIR")" != "$PUID:$PGID" ];
then
    NEEDS_CHOWN=1
fi
if [ -n "$ATTACHMENTS_DIR" ] && [ "$(stat -c '%u:%g' "$ATTACHMENTS_DIR")" != "$PUID:$PGID" ]; then
    NEEDS_CHOWN=1
fi

if [ "$NEEDS_CHOWN" = "1" ]; then
    chown -R appuser:appgroup "$DATA_DIR" "$PROFILE_PHOTO_DIR"
    if [ -n "$ATTACHMENTS_DIR" ]; then
        chown -R appuser:appgroup "$ATTACHMENTS_DIR"
    fi
fi

# Render the nginx-edge HSTS header (issue #364). nginx.conf includes
# /etc/nginx/hsts.conf in every add_header block, so this file must always
# exist; it is empty unless HSTS is enabled. HSTS is gated on the same
# COOKIE_SECURE signal the backend uses (backend/main.go -> SecurityHeadersMiddleware)
# so the two never disagree about whether TLS sits in front. The default
# docker-compose deployment is plain HTTP (7300:8080), where a blanket HSTS
# would make browsers refuse the app for the max-age duration.
: > /etc/nginx/hsts.conf
case "${COOKIE_SECURE:-false}" in
    1|t|T|true|TRUE|True)
        echo 'add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;' > /etc/nginx/hsts.conf
        ;;
esac

# Render the SPA Content-Security-Policy's map-tile origin (issue #1286
# follow-up). The contact map loads its style JSON, vector tiles and glyphs
# straight from the origin of MAP_TILE_STYLE_URL (ADR 0031), and MapLibre
# builds its worker from a blob: URL -- both must be allowed (the same-origin
# /service-worker.js too, hence worker-src 'self' blob:), or the map draws its
# pins on a blank canvas. The tile host is operator-configurable, so it
# cannot be hard-coded in nginx.conf: this renders /etc/nginx/csp_tile.conf,
# which docker/nginx.conf includes to define $csp_tile_origin, exactly like
# hsts.conf above. The fallback must stay in step with
# config.DefaultMapTileStyleURL (a drift test reads this file).
MAP_TILE_STYLE_URL="${MAP_TILE_STYLE_URL:-https://tiles.openfreemap.org/styles/liberty}"
# scheme://host, path stripped -- CSP sources are origins, not URLs.
_tile_rest="${MAP_TILE_STYLE_URL#*://}"
TILE_ORIGIN="${MAP_TILE_STYLE_URL%%://*}://${_tile_rest%%/*}"
printf 'set $csp_tile_origin "%s";\n' "$TILE_ORIGIN" > /etc/nginx/csp_tile.conf

exec "$@"
