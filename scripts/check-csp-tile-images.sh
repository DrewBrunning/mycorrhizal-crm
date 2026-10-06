#!/usr/bin/env bash
# Built-image check for the SPA CSP's map-tile origin (issue #1286 follow-up,
# ADR 0031). The Go tests prove the nginx configs and the render script are
# internally consistent; only a *started* image proves nginx accepts the
# rendered include and that MAP_TILE_STYLE_URL really reaches the served header
# in BOTH deployment shapes (the all-in-one image and the split frontend image).
#
# Usage: scripts/check-csp-tile-images.sh <all-in-one-image> <split-frontend-image>
#
# For each image: the default URL yields the OpenFreeMap origin, a custom URL
# (with a port, a path and a query) yields exactly that origin and nothing
# from the default, and an unusable URL (credentials) refuses to start.
set -euo pipefail

ALLINONE_IMAGE="${1:?all-in-one image tag}"
SPLIT_IMAGE="${2:?split frontend image tag}"

PORT="${CSP_CHECK_PORT:-18083}"
NET="csp-tile-check-$$"
CUSTOM_URL='https://tiles.example.org:8443/styles/x?key=1'
CUSTOM_ORIGIN='https://tiles.example.org:8443'
DEFAULT_ORIGIN='https://tiles.openfreemap.org'

# shellcheck disable=SC2329 # invoked via the EXIT trap
cleanup() {
    docker rm -f csp-tile-check csp-tile-backend >/dev/null 2>&1 || true
    docker network rm "$NET" >/dev/null 2>&1 || true
}
trap cleanup EXIT
docker network create "$NET" >/dev/null

fail=0

# run_image <image> <MAP_TILE_STYLE_URL or ""> -- starts csp-tile-check.
run_image() {
    local image="$1" url="$2"
    docker rm -f csp-tile-check >/dev/null 2>&1 || true
    local args=(-d --name csp-tile-check --network "$NET" -p "$PORT:8080")
    [ -z "$url" ] || args+=(-e "MAP_TILE_STYLE_URL=$url")
    if [ "$image" = "$ALLINONE_IMAGE" ]; then
        args+=(-e SQLITE_DB_PATH=/app/data/csp.db -e PROFILE_PHOTO_DIR=/app/static/photos
            -e JWT_SECRET_KEY=csp-check-secret-key-minimum-32-characters-long
            -e FRONTEND_URL="http://localhost:$PORT")
    fi
    docker run "${args[@]}" "$image" >/dev/null
}

served_csp() {
    local csp
    for _ in $(seq 1 40); do
        csp="$(curl -fsSI "http://127.0.0.1:$PORT/" 2>/dev/null | tr -d '\r' | grep -i '^content-security-policy:' || true)"
        if [ -n "$csp" ]; then
            printf '%s' "$csp"
            return 0
        fi
        sleep 1
    done
    return 1
}

expect_origin() {
    local label="$1" image="$2" url="$3" want="$4" unwanted="$5" csp
    run_image "$image" "$url"
    if ! csp="$(served_csp)"; then
        echo "::error::$label: no Content-Security-Policy served"
        docker logs csp-tile-check 2>&1 | tail -20
        fail=1
        return
    fi
    for need in "connect-src 'self' $want" "img-src 'self' data: blob: $want" "worker-src 'self' blob:"; do
        if [[ "$csp" != *"$need"* ]]; then
            echo "::error::$label: CSP lacks \"$need\": $csp"
            fail=1
        fi
    done
    if [ -n "$unwanted" ] && [[ "$csp" == *"$unwanted"* ]]; then
        echo "::error::$label: CSP still names $unwanted: $csp"
        fail=1
    fi
    [ "$fail" -ne 0 ] || echo "OK: $label -> $want"
}

expect_refusal() {
    local label="$1" image="$2" url="$3" code
    run_image "$image" "$url"
    code="$(docker wait csp-tile-check)"
    if [ "$code" = "0" ]; then
        echo "::error::$label: container started with an unusable MAP_TILE_STYLE_URL"
        fail=1
    else
        echo "OK: $label refused to start (exit $code)"
    fi
}

# The split image's nginx proxies to a host named `backend`; any reachable
# container with that alias lets it resolve (the SPA root never calls it).
docker run -d --name csp-tile-backend --network "$NET" --network-alias backend \
    --entrypoint sleep "$SPLIT_IMAGE" 600 >/dev/null

for entry in "all-in-one:$ALLINONE_IMAGE" "split frontend:$SPLIT_IMAGE"; do
    label="${entry%%:*}"
    image="${entry#*:}"
    expect_origin "$label default" "$image" "" "$DEFAULT_ORIGIN" ""
    expect_origin "$label custom" "$image" "$CUSTOM_URL" "$CUSTOM_ORIGIN" "$DEFAULT_ORIGIN"
    expect_refusal "$label credentialed URL" "$image" 'https://user:pw@tiles.example.org/styles/x'
done

exit "$fail"
