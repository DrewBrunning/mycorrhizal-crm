#!/usr/bin/env bash
# Idempotent setup: point this checkout's git hooks at the vendored,
# checked-in hooks in .githooks/, which mirror CI's linters and
# governance/drift checks locally. See CLAUDE.md's "Local pre-commit checks"
# section. Safe to re-run.
#
# core.hooksPath is a local git config setting (not committed). Run this once
# per clone — and again in every linked worktree of that clone, since a
# worktree can carry its own override that shadows the shared setting (see
# below).

set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

chmod +x .githooks/pre-commit .githooks/commit-msg
git config core.hooksPath .githooks

# A git worktree can carry its own core.hooksPath override in
# .git/worktrees/<name>/config.worktree that shadows the setting above for
# commits made inside it — some environments provision every worktree this
# way by default, pinned at the real (untracked) .git/hooks dir. If this run
# is itself inside such a linked worktree (git-dir != git-common-dir), set
# the override here too so a commit made in *this* worktree actually goes
# through .githooks rather than silently falling back to whatever (if
# anything) sits in the real hooks directory. --worktree config requires the
# worktreeConfig extension, which we enable if it isn't already (harmless,
# repo-local, and required either way for config.worktree to be honored).
if [ "$(git rev-parse --git-dir)" != "$(git rev-parse --git-common-dir)" ]; then
  git config extensions.worktreeConfig true
  git config --worktree core.hooksPath .githooks
fi

echo "Installed: core.hooksPath -> .githooks"
echo "  effective in this checkout: $(git config core.hooksPath)"
