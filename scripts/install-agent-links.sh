#!/usr/bin/env bash
# Idempotent setup: make every CLAUDE.md alias a symlink to the colocated
# AGENTS.md, the portable context file that coding agents load.
#
# Git stores each alias as a symlink (mode 120000). A checkout made with
# `core.symlinks=false` (some Windows and container filesystems) materializes
# it as a small text file containing the link target instead. This script
# recreates the real symlink on those checkouts so both AGENTS.md-aware and
# CLAUDE.md-aware tools resolve the same content.
#
# If the filesystem does not permit symlinks, it falls back to a one-line
# `@AGENTS.md` import and warns that the working tree differs from git. Both
# forms are accepted by the context-check gate that follows this stage.
#
# Safe to re-run.

set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

links=(
  CLAUDE.md
  backend/CLAUDE.md
  frontend/CLAUDE.md
  android/CLAUDE.md
  docs/CLAUDE.md
  .github/CLAUDE.md
  backend/database/CLAUDE.md
)

fixed=0
already=0
fallback=0

for link in "${links[@]}"; do
  dir="$(dirname "$link")"
  target="$dir/AGENTS.md"

  if [ ! -e "$target" ] && [ ! -L "$target" ]; then
    echo "error: $target is missing; cannot create the alias at $link" >&2
    exit 1
  fi

  if [ -L "$link" ] && [ "$(readlink "$link")" = "AGENTS.md" ]; then
    already=$((already + 1))
    continue
  fi

  if [ -e "$link" ] && [ ! -L "$link" ]; then
    # A regular file here is either git's materialized symlink (its content is
    # the link target) or an `@AGENTS.md` fallback this script wrote earlier.
    # Only replace those exact shapes; never clobber a real document.
    contents="$(cat "$link" 2>/dev/null || true)"
    if [ "$contents" != "AGENTS.md" ] && [ "$contents" != "@AGENTS.md" ]; then
      echo "warning: $link is a regular file that is not an alias; leaving it untouched" >&2
      continue
    fi
    rm -f "$link"
  fi

  if ln -s AGENTS.md "$link" 2>/dev/null; then
    fixed=$((fixed + 1))
  else
    printf '@AGENTS.md\n' > "$link"
    fallback=$((fallback + 1))
    echo "warning: could not create a symlink at $link; wrote an '@AGENTS.md' import instead." >&2
    echo "         The working tree now differs from git (which stores a symlink); re-run this" >&2
    echo "         script after enabling symlinks to restore it." >&2
  fi
done

echo "Agent context links: $already already correct, $fixed repaired, $fallback fallback"
for link in "${links[@]}"; do
  printf '  %-28s -> %s\n' "$link" "$(readlink "$link" 2>/dev/null || echo '(regular file)')"
done
