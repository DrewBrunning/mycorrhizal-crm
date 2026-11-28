#!/usr/bin/env bash
# List the release tags whose docs may be published (issue #2244).
#
# Prints every `v*` tag reachable from REF (default HEAD; the workflow passes
# main) or from a release/* branch, local or remote-tracking, once each. Those
# are the reviewed lines of history: main and release/* both require a pull
# request (the main-protection and release-branch-protection rulesets), and
# security backports are cherry-picked onto release/vX.Y.0 before tagging
# (SECURITY.md). Creating a `v*` tag is not restricted, so a tag on any other
# commit (an unmerged branch) is left out: it must never become the site root
# or run its docs/ tree through a Jekyll build.
#
# Usage: scripts/docs-release-tags.sh [REF]
# cmd/docsversions then picks which of these to publish.
set -euo pipefail

ref="${1:-HEAD}"
if ! git rev-parse --verify --quiet "${ref}^{commit}" >/dev/null; then
  echo "docs-release-tags: unknown ref '$ref'" >&2
  exit 2
fi

{
  git tag -l 'v*' --merged "$ref"
  git for-each-ref --format='%(refname)' 'refs/heads/release/' 'refs/remotes/*/release/*' |
    while IFS= read -r branch; do
      git tag -l 'v*' --merged "$branch"
    done
} | sort -u
