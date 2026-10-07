#!/usr/bin/env bash
#
# Git facts for the manual-gate attestation check (issue #1486).
#
#   manual-gate-facts.sh <release-commit> <attested-commit>...
#
# Prints a JSON object keyed by attested commit:
#
#   {"<sha>": {"ancestor": true|false, "changed": ["path", ...]}}
#
# `ancestor` is true when the commit exists in this checkout and is an
# ancestor of (or equal to) the release commit; `changed` lists the paths that
# differ between the two (empty when not an ancestor). cmd/manualgatecheck
# matches `changed` against each gate's watch_paths -- the git questions live
# here so the Go side stays pure and unit-testable. A commit the checkout does
# not know is reported as non-ancestor, never an error: the attestation then
# fails closed with a message naming it.
#
# Needs jq and a checkout with full history (release.yml's preflight uses
# fetch-depth: 0). Run from inside the repository.
#
# Exit 0: JSON printed. Exit 2: usage error or the release commit is unknown.

set -eu

if [ "$#" -lt 1 ]; then
  echo "usage: manual-gate-facts.sh <release-commit> [attested-commit...]" >&2
  exit 2
fi
head_commit="$1"
shift

if ! git cat-file -e "${head_commit}^{commit}" 2>/dev/null; then
  echo "manual-gate-facts.sh: release commit '${head_commit}' is not a commit in this checkout" >&2
  exit 2
fi

out='{}'
for sha in "$@"; do
  ancestor=false
  changed='[]'
  if git cat-file -e "${sha}^{commit}" 2>/dev/null && git merge-base --is-ancestor "$sha" "$head_commit" 2>/dev/null; then
    ancestor=true
    changed="$(git diff --name-only "$sha" "$head_commit" | jq -R . | jq -s -c .)"
  fi
  out="$(printf '%s' "$out" | jq -c --arg sha "$sha" --argjson ancestor "$ancestor" --argjson changed "$changed" '. + {($sha): {ancestor: $ancestor, changed: $changed}}')"
done
printf '%s\n' "$out"
