#!/usr/bin/env bash
#
# Pull a docker image with a bounded retry/backoff (issue #1550). Registries
# rate-limit CI runners ("toomanyrequests"); a bare `docker run` pulls once and
# fails the whole leg. Callers then `docker run --pull never` so no second,
# unretried pull can happen.
#
# Usage: docker-pull-retry.sh <image>
set -euo pipefail

image="${1:?usage: docker-pull-retry.sh <image>}"
for attempt in 1 2 3 4; do
  docker pull --quiet "$image" >/dev/null && exit 0
  echo "::warning::pull of $image attempt $attempt failed; retrying in $((attempt * 20))s" >&2
  sleep $((attempt * 20))
done
exit 1
