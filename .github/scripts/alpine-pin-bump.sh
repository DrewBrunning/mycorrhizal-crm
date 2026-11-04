#!/usr/bin/env bash
#
# Bump the exact `pkg=version` pins in each Dockerfile's `RUN apk add` block to
# the version the Dockerfile's final-stage base image currently indexes.
#
# Alpine publishes a new package version and prunes the old one from the index
# at the same moment, so a pin that was valid yesterday fails every cold build
# today ("breaks: world[...]", issues #1062/#1131). The pins are deliberately
# exact (hadolint DL3018), so the fix is always a bump; this script makes that
# bump mechanical and is run on a schedule by alpine-pin-bump.yml, which opens
# the PR.
#
# Usage: alpine-pin-bump.sh Dockerfile...
#
# For every pin that moved it rewrites the pin in place, inserts a dated
# comment directly above the RUN line, and prints `<file>: <pkg> <old> -> <new>`
# on stdout. Silent and byte-identical when nothing moved. A package the index
# no longer lists (or whose search comes back empty) is warned about on stderr and left alone (the cold build
# fails loudly on it, which needs a human). Base-image digests are not touched.
#
# The base image is pulled through docker-pull-retry.sh (registry rate limits,
# #1550). ALPINE_PIN_BUMP_DOCKER, ALPINE_PIN_BUMP_PULL and ALPINE_PIN_BUMP_DATE
# override the docker binary, the pull helper and the comment date (for tests).

set -euo pipefail

DOCKER="${ALPINE_PIN_BUMP_DOCKER:-docker}"
TODAY="${ALPINE_PIN_BUMP_DATE:-$(date -u +%F)}"
PULL="${ALPINE_PIN_BUMP_PULL:-$(dirname "${BASH_SOURCE[0]}")/docker-pull-retry.sh}"

# Image ref of the last FROM line, with --platform=... style flags and `AS x`
# dropped.
final_image() {
  awk '
    /^FROM[[:space:]]/ {
      img = ""
      for (i = 2; i <= NF; i++) {
        if ($i ~ /^--/) continue
        img = $i
        break
      }
    }
    END { print img }
  ' "$1"
}

# "<line-number> <pkg=version ...>" for the final `RUN apk add` block (the
# first line of the block and every pin in it, across `\` continuations).
apk_block() {
  awk '
    /^FROM[[:space:]]/ { start = 0; pins = "" ; inblock = 0 }
    /^RUN[[:space:]]+apk[[:space:]]+add[[:space:]]/ { start = NR; inblock = 1 }
    inblock {
      for (i = 1; i <= NF; i++) {
        if ($i ~ /^[a-z0-9][a-z0-9._+-]*=[A-Za-z0-9._~-]+$/) pins = pins " " $i
      }
      if ($0 !~ /\\[[:space:]]*$/) inblock = 0
    }
    END { if (start) print start pins }
  ' "$1"
}

escape_re() { printf '%s' "$1" | sed 's/[][\.*^$+/]/\\&/g'; }

for file in "$@"; do
  block="$(apk_block "$file")"
  [ -n "$block" ] || continue
  run_line="${block%% *}"
  read -r -a pins <<<"${block#* }"

  image="$(final_image "$file")"
  names=()
  for pin in "${pins[@]}"; do names+=("${pin%%=*}"); done

  # One `apk search -e` per name, printed as "<name> <name-version>": the match
  # can be a different package than the one named (libc6-compat is provided by
  # gcompat, whose version it tracks).
  "$PULL" "$image"
  # shellcheck disable=SC2016 # the loop runs inside the container; $@/$n must not expand here
  index="$("$DOCKER" run --rm --pull never --user root --entrypoint sh "$image" -c \
    'apk update >/dev/null && for n in "$@"; do printf "%s %s\n" "$n" "$(apk search -e "$n" | head -n1)"; done' \
    sh "${names[@]}")"

  changes=()
  for pin in "${pins[@]}"; do
    name="${pin%%=*}"
    old="${pin#*=}"
    new=""
    while read -r queried entry; do
      if [ "$queried" = "$name" ] && [[ "$entry" =~ -([0-9][^-]*-r[0-9]+)$ ]]; then
        new="${BASH_REMATCH[1]}"
        break
      fi
    done <<<"$index"
    if [ -z "$new" ]; then
      echo "WARNING: $file: $name is not in $image's index; pin left at $old" >&2
      continue
    fi
    [ "$new" != "$old" ] || continue
    sed -i "s/\\b$(escape_re "$name")=$(escape_re "$old")\\b/$name=$new/" "$file"
    changes+=("$name $old -> $new")
    echo "$file: $name $old -> $new"
  done

  if [ "${#changes[@]}" -gt 0 ]; then
    joined="$(printf '%s, ' "${changes[@]}")"
    sed -i "${run_line}i # $TODAY (alpine-pin-bump, automated): ${joined%, }. Alpine pruned the old version from the index." "$file"
  fi
done
