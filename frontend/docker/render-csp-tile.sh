#!/bin/sh
# Renders the nginx include that defines $csp_tile_origin, the one operator-
# configurable origin in the SPA Content-Security-Policy (issue #1286 follow-up,
# ADR 0031). The contact map loads its style JSON, vector tiles and glyphs
# straight from the origin of MAP_TILE_STYLE_URL, so that origin must be in
# connect-src / img-src.
#
# Shared by BOTH images -- the all-in-one (docker/entrypoint.sh) and the split
# frontend (nginx's /docker-entrypoint.d/) -- so they cannot drift.
#
# Usage: render-csp-tile.sh [output-path]
#   output-path defaults to $CSP_TILE_CONF_PATH, then /etc/nginx/csp_tile.conf.
#
# The value is parsed strictly rather than trimmed with shell expansion: only
# scheme://host[:port] is ever emitted. Userinfo, query, fragment and path are
# dropped; anything that is not an http(s) URL with a plain host fails the
# container start loudly instead of writing a broken or injectable nginx
# directive (a blank basemap is a silent failure; a refused start is not).
#
# The fallback must stay in step with config.DefaultMapTileStyleURL (a drift
# test reads this file).
set -eu

out="${1:-${CSP_TILE_CONF_PATH:-/etc/nginx/csp_tile.conf}}"
url="${MAP_TILE_STYLE_URL:-}"
# Same trimming as config.EffectiveMapTileStyleURL: blank means "use default".
url="$(printf '%s' "$url" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
[ -n "$url" ] || url="https://tiles.openfreemap.org/styles/liberty"

# scheme://host[:port], then an optional /path, ?query or #fragment (dropped).
# Host is a DNS name / IPv4 ([A-Za-z0-9.-]) or a bracketed IPv6 literal; '@'
# (userinfo), quotes, ';', '$', '{', '}' and whitespace never match.
origin="$(printf '%s' "$url" | sed -nE \
    's~^(https?)://([A-Za-z0-9.-]+|\[[0-9A-Fa-f:.]+\])(:[0-9]{1,5})?([/?#].*)?$~\1://\2\3~p')"

if [ -z "$origin" ] || [ "$(printf '%s' "$url" | wc -l)" != "0" ]; then
    echo "MAP_TILE_STYLE_URL must be an absolute http(s) URL with a plain host and no credentials, e.g. https://tiles.openfreemap.org/styles/liberty (got: $url)" >&2
    exit 1
fi

# shellcheck disable=SC2016 # the literal $csp_tile_origin is nginx's variable, not the shell's
printf 'set $csp_tile_origin "%s";\n' "$origin" > "$out"
