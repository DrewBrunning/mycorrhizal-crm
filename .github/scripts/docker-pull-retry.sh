#!/usr/bin/env bash
#
# Pull a docker image with a bounded retry/backoff (issue #1550). Registries
# rate-limit CI runners ("toomanyrequests"); a bare `docker run` pulls once and
# fails the whole leg. Callers then `docker run --pull never` so no second,
# unretried pull can happen.
#
# Usage: docker-pull-retry.sh <image>
# DOCKER_PULL_RETRY_STEP (default 20) is the per-attempt backoff in seconds;
# tests set it to 0.
set -euo pipefail

image="${1:?usage: docker-pull-retry.sh <image>}"
for attempt in 1 2 3 4; do
  docker pull --quiet "$image" >/dev/null && exit 0
  step="${DOCKER_PULL_RETRY_STEP:-20}"
  echo "::warning::pull of $image attempt $attempt failed; retrying in $((attempt * step))s" >&2
  sleep $((attempt * step))
done
exit 1
