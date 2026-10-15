#!/usr/bin/env bash
#
# Issue #1713: decides whether a "Real-release data vX.Y.Z -> current" leg
# should run now or be skipped because the release's image cannot exist yet.
#
# Why: release.yml's release commit registers the new tag in
# internal/schemafixture.SupportedReleases and pushes to main; that push runs
# migration-tests.yml, whose matrix (cmd/releaselist) includes the tag being
# cut. The tag-triggered docker-publish.yml has not pushed the image yet, so
# the leg's pull failed with "manifest unknown" and raised a false p1
# "Main failure: Migration Tests". The new release's leg now runs from
# docker-publish.yml after publication (real-release-leg.yml); here it only
# has to not fail early.
#
# Usage: realrelease-image-expected.sh <vX.Y.Z> [releases.go path]
# Run from inside the git checkout (needs HEAD and HEAD^).
#
# Prints RUN or SKIP on stdout (also writes skip=true|false to $GITHUB_OUTPUT
# when set). A SKIP is announced with ::notice::. Exit 0 always.
#
# SKIP only when BOTH hold:
#   1. the image definitively does not exist: the manifest lookup fails with a
#      "manifest unknown"-class answer (a rate limit / network error is NOT
#      proof of absence and stays RUN, so the leg's own retries and honest
#      failure apply); and
#   2. the tag is not yet published *by construction*: either HEAD is the
#      release-registration commit (it adds the tag's `{Tag: "...",` line to
#      releases.go), or the tag's docker-publish.yml run has not completed.
# Any other missing image -- a published release whose image is gone -- is RUN,
# and the leg then fails as it should.
#
# Test seams (never set in CI): REALRELEASE_MANIFEST_CMD (default
# `docker manifest inspect`), REALRELEASE_PUBLISH_PENDING_CMD (default queries
# the Actions API; prints the number of non-completed docker-publish runs).
set -u

tag="${1:?usage: realrelease-image-expected.sh vX.Y.Z [releases.go]}"
releases="${2:-backend/internal/schemafixture/releases.go}"
repo="${REALRELEASE_IMAGE:-ghcr.io/drewbrunning/mycorrhizal-crm}"
image="${repo}:${tag#v}"

emit() {
  echo "$1"
  if [ -n "${GITHUB_OUTPUT:-}" ]; then
    if [ "$1" = SKIP ]; then echo "skip=true" >> "$GITHUB_OUTPUT"; else echo "skip=false" >> "$GITHUB_OUTPUT"; fi
  fi
}

manifest_cmd="${REALRELEASE_MANIFEST_CMD:-docker manifest inspect}"
# shellcheck disable=SC2086
if out="$($manifest_cmd "$image" 2>&1)"; then
  emit RUN
  exit 0
fi
if ! printf '%s' "$out" | grep -qiE 'manifest unknown|no such manifest|name unknown|not found'; then
  emit RUN # inconclusive (rate limit, network): let the leg's retries decide.
  exit 0
fi

reason=""
if git diff HEAD^ HEAD -U0 -- "$releases" 2>/dev/null | grep -qE "^\+[[:space:]]*\{Tag: \"${tag}\","; then
  reason="HEAD is the release-registration commit for ${tag}"
else
  pending_cmd="${REALRELEASE_PUBLISH_PENDING_CMD:-}"
  if [ -n "$pending_cmd" ]; then
    pending="$($pending_cmd "$tag" 2>/dev/null || echo 0)"
  else
    pending="$(gh api "repos/${GITHUB_REPOSITORY:-}/actions/workflows/docker-publish.yml/runs?head_branch=${tag}&per_page=20" \
      --jq '[.workflow_runs[] | select(.status != "completed")] | length' 2>/dev/null || echo 0)"
  fi
  if [ "${pending:-0}" -gt 0 ] 2>/dev/null; then
    reason="docker-publish.yml for ${tag} has not completed"
  fi
fi

if [ -n "$reason" ]; then
  echo "::notice::skipping the real-release leg for ${tag}: ${image} is not published yet (${reason}); docker-publish.yml runs this leg after it publishes (issue #1713)"
  emit SKIP
else
  emit RUN
fi
