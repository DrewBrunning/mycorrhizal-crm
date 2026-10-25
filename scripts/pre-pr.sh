#!/usr/bin/env bash
# Local pre-PR gate (issue #1964). Selects the CI areas a branch touches from
# .github/filters.yaml and runs the matching checks.
#
#   scripts/pre-pr.sh --fast      # lint/build/drift checks for the touched areas
#   scripts/pre-pr.sh --focused   # + tests, coverage, per-file ratchets (recommended)
#   scripts/pre-pr.sh --focused --fix  # write-mode formatters/linters first
#   scripts/pre-pr.sh --full      # + exhaustive flake stress
#
# --focused is the normal pre-PR gate described in
# docs/development/implementation-workflow.md: it catches failing tests, changed-
# line coverage, and per-file ratchet regressions without the expensive flake-
# stress pass. --full adds that pass. Neither replaces CI. Open the PR and stop;
# do not poll `gh pr checks`.
#
# Works from the repository root, from backend/, or from any worktree.
# Extra flags are passed through (e.g. --base origin/main, --dry-run).
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel 2>/dev/null || true)"
if [ -z "$repo_root" ]; then
	echo "pre-pr: not inside a git repository" >&2
	exit 2
fi

cd "$repo_root/backend"
exec go run ./cmd/prepr --root "$repo_root" "$@"
