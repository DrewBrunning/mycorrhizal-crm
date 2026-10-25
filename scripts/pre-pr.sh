#!/usr/bin/env bash
# Local pre-PR gate (issue #1964). Selects the CI areas a branch touches from
# .github/filters.yaml and runs the matching checks.
#
#   scripts/pre-pr.sh --fast   # lint/build/drift checks for the touched areas
#   scripts/pre-pr.sh --full   # + area suites, coverage ratchets, flake stress
#
# --full is the pre-PR gate described in docs/development/implementation-workflow.md;
# it includes the coverage ratchet and flake-stress work the pre-commit hook
# deliberately leaves out. It does not replace CI. Open the PR and stop; do not
# poll `gh pr checks`.
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
