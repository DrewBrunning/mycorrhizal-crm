#!/usr/bin/env bash
# Issue #1484: helpers for the build-once release image. release-validate.yml's
# `build-candidate` job builds the all-in-one image ONCE and pushes it to GHCR
# under a non-release `candidate-<sha>` tag; every image-running gate pulls it
# BY DIGEST (never the mutable tag) and docker-publish.yml re-tags that exact
# digest for the release instead of rebuilding.
#
# Subcommands:
#   check-digest <digest>
#       Exit 0 iff <digest> is a well-formed `sha256:<64 hex>`. A malformed or
#       empty value (an unset reusable-workflow output) is rejected so a gate
#       can never silently fall through to something else.
#   pull <digest> <local-tag>
#       docker pull <repo>@<digest>, tag it <local-tag> (the name
#       docker-compose.test.yml / the hardening scan expect), and fail unless
#       the pulled image is exactly <digest>. Logs the digest so every gate's
#       log shows the same one, and exports MYCORRHIZAL_IMAGE=<repo>@<digest>
#       to $GITHUB_ENV when set. <repo> is $CANDIDATE_IMAGE_REPO or
#       ghcr.io/<lowercased $GITHUB_REPOSITORY>.
#   assert-match <tested-digest> <published-digest> <what>
#       Exit 1 with an ::error:: unless the two digests are equal -- the
#       "published digest == tested digest" assertion in verify-release-assets.
set -euo pipefail

digest_re='^sha256:[0-9a-f]{64}$'

check_digest() {
  [[ "${1:-}" =~ $digest_re ]]
}

repo_ref() {
  if [ -n "${CANDIDATE_IMAGE_REPO:-}" ]; then
    printf '%s' "$CANDIDATE_IMAGE_REPO"
  elif [ -n "${GITHUB_REPOSITORY:-}" ]; then
    printf 'ghcr.io/%s' "${GITHUB_REPOSITORY,,}"
  else
    return 1
  fi
}

cmd="${1:-}"
shift || true
case "$cmd" in
  check-digest)
    if ! check_digest "${1:-}"; then
      echo "::error::'${1:-}' is not a sha256 image digest" >&2
      exit 1
    fi
    ;;
  pull)
    digest="${1:-}"
    local_tag="${2:-}"
    if ! check_digest "$digest"; then
      echo "::error::refusing to pull: '$digest' is not a sha256 image digest" >&2
      exit 1
    fi
    if [ -z "$local_tag" ]; then
      echo "::error::pull needs a local tag to apply" >&2
      exit 2
    fi
    repo="$(repo_ref)" || { echo "::error::set CANDIDATE_IMAGE_REPO or GITHUB_REPOSITORY" >&2; exit 2; }
    ref="${repo}@${digest}"
    "$(dirname "$0")/docker-pull-retry.sh" "$ref"
    docker tag "$ref" "$local_tag"
    # Belt and braces: the registry returned what we asked for. A pull by
    # digest cannot return anything else, but the whole point of this gate is
    # that the tested artifact is provably the named one.
    have="$(docker image inspect --format '{{json .RepoDigests}}' "$local_tag")"
    if ! grep -qF "@${digest}" <<<"$have"; then
      echo "::error::pulled image does not carry digest ${digest} (RepoDigests: ${have})" >&2
      exit 1
    fi
    echo "Candidate image under test: ${ref} (tagged locally as ${local_tag})"
    # Later steps that boot the image through Compose by reference (deploy-smoke's
    # docker-compose.candidate.yml) read it from here.
    if [ -n "${GITHUB_ENV:-}" ]; then
      echo "MYCORRHIZAL_IMAGE=${ref}" >> "$GITHUB_ENV"
    fi
    if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
      echo "Candidate image under test: \`${ref}\`" >> "$GITHUB_STEP_SUMMARY"
    fi
    ;;
  assert-match)
    tested="${1:-}"
    published="${2:-}"
    what="${3:-image}"
    if ! check_digest "$tested" || ! check_digest "$published"; then
      echo "::error::cannot compare ${what}: tested='${tested}' published='${published}' (both must be sha256 digests)" >&2
      exit 1
    fi
    if [ "$tested" != "$published" ]; then
      echo "::error::${what}: published digest ${published} != tested digest ${tested} -- the image that shipped is not the image the gates tested" >&2
      exit 1
    fi
    echo "ok: ${what} published digest == tested digest (${published})"
    ;;
  *)
    echo "usage: candidate-image.sh check-digest <digest> | pull <digest> <local-tag> | assert-match <tested> <published> <what>" >&2
    exit 2
    ;;
esac
