#!/usr/bin/env bash
#
# Tests for ../alpine-pin-bump.sh.
#
# The script rewrites `pkg=version` pins in a Dockerfile's `RUN apk add` block
# to whatever the final stage's base image currently indexes. `docker` is
# replaced with a shim that records the image it was asked to run and answers
# `apk search -e` from a canned index, so the tests need no network.

# SC2319: `rc=$?` deliberately captures the status of the preceding `[ ... ]` test.
# shellcheck disable=SC2319
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$SCRIPT_DIR/../alpine-pin-bump.sh"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

cat >"$WORK/docker" <<'SHIM'
#!/usr/bin/env bash
# args: run --rm --pull never --user root --entrypoint sh IMAGE -c SCRIPT sh NAME...
# Index lines are "<pkg>-<version>" (a package) or "<virtual>><pkg>-<version>"
# (a virtual name that package provides). Prints "<name> <entry>" per NAME,
# like the in-container loop does; entry is empty when nothing matches.
printf '%s\n' "$9" >>"$FAKE_DOCKER_LOG"
[ -z "${FAKE_DOCKER_FAIL:-}" ] || exit 7
shift 11
for n in "$@"; do
  hit="$(grep -m1 -E "^$n-[0-9]" "$FAKE_INDEX")"
  [ -n "$hit" ] || hit="$(grep -m1 "^$n>" "$FAKE_INDEX" | cut -d'>' -f2)"
  printf '%s %s\n' "$n" "$hit"
done
SHIM
chmod +x "$WORK/docker"

export ALPINE_PIN_BUMP_DOCKER="$WORK/docker"
export ALPINE_PIN_BUMP_PULL=true
export ALPINE_PIN_BUMP_DATE="2026-11-04"
export FAKE_DOCKER_LOG="$WORK/docker.log"
export FAKE_INDEX="$WORK/index"

pass=0
fail=0

check() { # $1 = name, $2 = condition exit code (0 = pass), $3 = detail
  if [ "$2" -eq 0 ]; then
    pass=$((pass + 1))
    echo "PASS: $1"
  else
    fail=$((fail + 1))
    echo "FAIL: $1 ($3)"
  fi
}

reset() {
  : >"$FAKE_DOCKER_LOG"
  unset FAKE_DOCKER_FAIL
  printf '%s\n' tzdata-2026f-r0 nginx-1.30.6-r0 zlib-1.3.2-r1 >"$FAKE_INDEX"
}

multiline() {
  cat >"$1" <<'DF'
FROM --platform=$BUILDPLATFORM golang:1.27-alpine@sha256:aaa AS builder
RUN apk add --no-cache git=2.0-r0
FROM alpine:3.24@sha256:bbb
ENV TZ=UTC
RUN apk add --no-cache \
    tzdata=2026e-r0 \
    nginx=1.30.6-r0 \
    zlib=1.3.2-r1 \
    && rm -f /var/log/apk.log
WORKDIR /app
DF
}

# --- stale pin is bumped, others untouched, comment inserted, summary printed
reset
multiline "$WORK/a"
out="$(bash "$SCRIPT" "$WORK/a" 2>&1)"
code=$?
check "exit 0 on bump" "$code" "got $code: $out"
grep -qF "    tzdata=2026f-r0 \\" "$WORK/a"
rc=$?; check "tzdata pin rewritten" "$rc" "$(cat "$WORK/a")"
grep -q 'nginx=1.30.6-r0' "$WORK/a" && grep -q 'zlib=1.3.2-r1' "$WORK/a"
rc=$?; check "current pins untouched" "$rc" "$(cat "$WORK/a")"
grep -q 'ENV TZ=UTC' "$WORK/a"
rc=$?; check "non-apk key=value lines untouched" "$rc" "$(cat "$WORK/a")"
grep -q 'git=2.0-r0' "$WORK/a"
rc=$?; check "earlier stage's apk add untouched" "$rc" "$(cat "$WORK/a")"
[ "$(grep -n 'alpine-pin-bump' "$WORK/a" | cut -d: -f1)" = "$(($(grep -n '^RUN apk add --no-cache \\$' "$WORK/a" | cut -d: -f1) - 1))" ]
rc=$?; check "dated comment sits directly above the final-stage RUN" "$rc" "$(cat "$WORK/a")"
grep -q '2026-11-04 (alpine-pin-bump, automated): tzdata 2026e-r0 -> 2026f-r0' "$WORK/a"
rc=$?; check "comment names the bump" "$rc" "$(cat "$WORK/a")"
printf '%s\n' "$out" | grep -qF "$WORK/a: tzdata 2026e-r0 -> 2026f-r0"
rc=$?; check "summary line on stdout" "$rc" "$out"
[ "$(cat "$FAKE_DOCKER_LOG")" = "alpine:3.24@sha256:bbb" ]
rc=$?; check "queries the FINAL stage's image, flags stripped" "$rc" "$(cat "$FAKE_DOCKER_LOG")"

# --- single-line apk add
reset
cat >"$WORK/b" <<'DF'
FROM alpine:3.24@sha256:bbb
RUN apk add --no-cache tzdata=2026e-r0 zlib=1.3.2-r1
DF
bash "$SCRIPT" "$WORK/b" >/dev/null 2>&1
grep -q 'RUN apk add --no-cache tzdata=2026f-r0 zlib=1.3.2-r1' "$WORK/b"
rc=$?; check "single-line apk add is bumped" "$rc" "$(cat "$WORK/b")"

# --- up to date: byte-identical, silent
reset
multiline "$WORK/c"
sed -i 's/2026e-r0/2026f-r0/' "$WORK/c"
cp "$WORK/c" "$WORK/c.orig"
out="$(bash "$SCRIPT" "$WORK/c" 2>&1)"
cmp -s "$WORK/c" "$WORK/c.orig"
rc=$?; check "no-op leaves file byte-identical" "$rc" "$(diff "$WORK/c.orig" "$WORK/c")"
[ -z "$out" ]
rc=$?; check "no-op prints nothing" "$rc" "$out"

# --- package missing from the index: warn, leave pin, exit 0
reset
printf '%s\n' nginx-1.30.6-r0 zlib-1.3.2-r1 >"$FAKE_INDEX"
multiline "$WORK/d"
cp "$WORK/d" "$WORK/d.orig"
out="$(bash "$SCRIPT" "$WORK/d" 2>&1)"
code=$?
cmp -s "$WORK/d" "$WORK/d.orig"
rc=$?; check "package absent from index keeps its pin" "$rc" "$(diff "$WORK/d.orig" "$WORK/d")"
[ "$code" -eq 0 ] && printf '%s\n' "$out" | grep -q 'WARNING.*tzdata'
rc=$?; check "absent package warns, exits 0" "$rc" "code=$code out=$out"

# --- a virtual name resolves to the package that provides it
reset
printf '%s\n' 'libc6-compat>gcompat-1.1.0-r5' libssl3-extra-9.9-r0 libssl3-3.5.9-r0 >"$FAKE_INDEX"
cat >"$WORK/e" <<'DF'
FROM alpine:3.24@sha256:bbb
RUN apk add --no-cache libc6-compat=1.1.0-r4 libssl3=3.5.8-r0
DF
bash "$SCRIPT" "$WORK/e" >/dev/null 2>&1
grep -q 'libc6-compat=1.1.0-r5 libssl3=3.5.9-r0' "$WORK/e"
rc=$?; check "virtual provides and prefix-sharing names resolve exactly" "$rc" "$(cat "$WORK/e")"

# --- docker failure surfaces as a non-zero exit
reset
multiline "$WORK/f"
FAKE_DOCKER_FAIL=1 bash "$SCRIPT" "$WORK/f" >/dev/null 2>&1
code=$?
[ "$code" -ne 0 ]
rc=$?; check "docker failure fails the script" "$rc" "exit $code"

# --- a Dockerfile with no apk add is skipped without touching docker
reset
printf 'FROM alpine:3.24@sha256:bbb\nRUN echo hi\n' >"$WORK/g"
bash "$SCRIPT" "$WORK/g" >/dev/null 2>&1
code=$?
[ "$code" -eq 0 ] && [ ! -s "$FAKE_DOCKER_LOG" ]
rc=$?; check "no apk add: exit 0, docker not called" "$rc" "code=$code log=$(cat "$FAKE_DOCKER_LOG")"

echo "passed=$pass failed=$fail"
[ "$fail" -eq 0 ]
