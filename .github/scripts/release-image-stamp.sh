#!/usr/bin/env bash
# Issue #1484: the ONE definition of the release image's build stamp, shared by
# release-validate.yml's `build-candidate` job (which builds the image every
# gate then tests) and docker-publish.yml's `build-and-push` job (whose
# from-source fallback builds the same image when no tested candidate exists,
# i.e. an operator override). Two copies of this arithmetic would let the
# candidate and the published image disagree on APP_VERSION / APP_COMMIT /
# APP_BUILD_DATE / SOURCE_DATE_EPOCH -- and then "the image the gates tested is
# the image that ships" would be a claim about two different images.
#
# Usage (from inside a checkout of the commit being built):
#   release-image-stamp.sh <release-tag>
# Emits `key=value` lines to $GITHUB_OUTPUT when set, else to stdout:
#   commit             12-char commit prefix (buildinfo's VCS fallback length)
#   sha7               7-char prefix, the extra image tag
#   sha_full           full SHA, for the OCI revision label
#   source_date_epoch  the commit's committer date (REL-04, #448: deterministic
#                      per commit so BuildKit timestamps are reproducible)
#   created            that epoch as RFC 3339 UTC, for the OCI created label
#   display_version    version the binary reports: strips `v` and everything
#                      from the first `-`, so an RC reports the final it will
#                      become (issue #1022); the image TAG keeps the full -rc.N
set -euo pipefail

tag="${1:-}"
if ! printf '%s' "$tag" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+(-rc\.[0-9]+)?$'; then
  echo "::error::release-image-stamp.sh: '$tag' is not a release tag (vMAJOR.MINOR.PATCH, optional -rc.N)" >&2
  exit 2
fi

sha="$(git rev-parse HEAD)"
epoch="$(git log -1 --format=%ct HEAD)"
v="${tag#v}"

out="${GITHUB_OUTPUT:-/dev/stdout}"
{
  echo "commit=$(echo "$sha" | cut -c1-12)"
  echo "sha7=$(echo "$sha" | cut -c1-7)"
  echo "sha_full=$sha"
  echo "source_date_epoch=$epoch"
  echo "created=$(date -u -d "@$epoch" +%Y-%m-%dT%H:%M:%SZ)"
  echo "display_version=${v%%-*}"
} >> "$out"
