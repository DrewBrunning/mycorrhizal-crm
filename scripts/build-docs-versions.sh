#!/usr/bin/env bash
# Build the versioned documentation site (issue #2244).
#
# One Jekyll build per entry of `cmd/docsversions` (the newest patch of each
# released minor at or above the floor, plus main as "next"), assembled into one
# Pages artifact: the newest release at the site root, older ones under /X.Y/,
# the unreleased docs under /next/, and versions.json for the switcher in
# docs/_includes/header_custom.html. Each release is built from its own tag, so
# a release's pages are the ones that shipped with it; only the switcher include
# and the Changelog page are taken from the current tree so every version has them,
# and cmd/docssitelinks fixes each version's dead relative links after its build.
#
# Usage (from the repo root, with the docs bundle installed and full tag history):
#   PAGES_BASE_PATH=/mycorrhizal-crm scripts/build-docs-versions.sh [out-dir]
# out-dir defaults to docs-site/. DOCS_NEXT_REF (default HEAD) is the ref published
# as /next/ (the workflow passes main). PAGES_BASE_PATH is the Pages base path
# with no trailing slash (empty for a custom domain).
#
# Only tags on main or a release/* branch are candidates
# (scripts/docs-release-tags.sh): a tag pushed on an unreviewed commit is never
# built or published.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
out="${1:-$repo_root/docs-site}"
base="${PAGES_BASE_PATH:-}"
base="${base%/}"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

cd "$repo_root"
versions="$work/versions.json"
next_ref="${DOCS_NEXT_REF:-HEAD}"
scripts/docs-release-tags.sh "$next_ref" | (cd backend && go run ./cmd/docsversions) > "$versions"
# The newest *release*: before the first one, main is the whole site and the
# banner must not link "the latest release" back to itself (YAML ~ is nil,
# which Liquid treats as false; an empty string would be true).
latest_label="$(jq -r '[.[] | select(.latest and (.unreleased | not))][0].label // "~"' "$versions")"
[ "$latest_label" = "~" ] || latest_label="\"$latest_label\""

mkdir -p "$work/site"
export BUNDLE_GEMFILE="$repo_root/docs/Gemfile"
export JEKYLL_ENV=production

while IFS= read -r entry; do
  tag="$(jq -r .tag <<<"$entry")"
  label="$(jq -r .label <<<"$entry")"
  vpath="$(jq -r .path <<<"$entry")"
  unreleased="$(jq -r '.unreleased // false' <<<"$entry")"
  src="$work/src-$label"
  mkdir -p "$src"
  if [ "$tag" = main ]; then
    # The unreleased docs are the checked-out tree (a merge to main).
    git archive "$next_ref" docs CHANGELOG.md changelog.d | tar -x -C "$src"
    # Unreleased entries wait in changelog.d/ until a release assembles them,
    # so CHANGELOG.md's [Unreleased] is empty between releases. Fold them into
    # this temporary copy so /next/ shows what is coming.
    (cd backend && go run ./cmd/changelogassemble -write -file "$src/CHANGELOG.md" -dir "$src/changelog.d" >/dev/null)
  else
    git archive "$tag" docs CHANGELOG.md 2>/dev/null | tar -x -C "$src" \
      || git archive "$tag" docs | tar -x -C "$src"
  fi
  # Every version gets the current switcher, and the Changelog page when the
  # tag has a CHANGELOG.md (the page itself is generated, never committed).
  mkdir -p "$src/docs/_includes"
  cp "$repo_root/docs/_includes/header_custom.html" "$src/docs/_includes/header_custom.html"
  if [ -f "$src/CHANGELOG.md" ]; then
    # Repository links name the tag the page was built from (main for next).
    (cd backend && go run ./cmd/gensitechangelog -in "$src/CHANGELOG.md" -out "$src/docs/changelog.md" -ref "$tag")
  fi
  vbase="$base"
  [ -n "$vpath" ] && vbase="$base/${vpath%/}"
  cat > "$src/docs/_config_version.yml" <<YAML
baseurl: "$vbase"
docs_root: "$base"
docs_version_label: "$label"
docs_version_path: "$vpath"
docs_latest_label: $latest_label
docs_unreleased: $unreleased
YAML
  bundle exec jekyll build \
    --source "$src/docs" \
    --destination "$work/site/${vpath:-_root}" \
    --config "$src/docs/_config.yml,$src/docs/_config_version.yml"
  # No dead links in any version: a .md link to a rendered page gets its .html
  # address, and a repository file or unpublished record points at GitHub at
  # the ref this version was built from (cmd/docssitelinks). Older tags shipped
  # before the docs used .html links, so this runs on every version.
  (cd backend && go run ./cmd/docssitelinks -site "$work/site/${vpath:-_root}" -src "$src/docs" -ref "$tag")
done < <(jq -c '.[]' "$versions")

mkdir -p "$out"
find "$out" -mindepth 1 -delete
cp -a "$work/site/_root/." "$out/"
for d in "$work"/site/*/; do
  name="$(basename "$d")"
  [ "$name" = _root ] && continue
  mkdir -p "$out/$name"
  cp -a "$d/." "$out/$name/"
done
cp "$versions" "$out/versions.json"
touch "$out/.nojekyll"
echo "Built $(jq length "$versions") docs version(s) into $out"
