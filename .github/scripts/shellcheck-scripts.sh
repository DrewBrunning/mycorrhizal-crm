#!/usr/bin/env bash
# Issue #1904: shellcheck every tracked shell script.
#
# The script list is discovered, never hand-maintained, so a new script is in
# scope the moment it is committed:
#   - every `git ls-files '*.sh'`, plus
#   - every extension-less tracked file whose shebang selects sh/bash (that is
#     how .githooks/* and android/gradlew are found).
#
# `-x` follows sourced files (the alert scripts source alert-lib/issues.sh,
# which is otherwise SC1091).
#
# Usage:
#   ./.github/scripts/shellcheck-scripts.sh            # all tracked (CI)
#   ./.github/scripts/shellcheck-scripts.sh --staged   # staged (pre-commit)
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

staged=false
case "${1:-}" in
  "") ;;
  --staged) staged=true ;;
  *)
    echo "usage: $0 [--staged]" >&2
    exit 2
    ;;
esac

is_shell_script() {
  local file="$1" first
  case "$file" in
    *.sh) return 0 ;;
  esac
  # Extension-less only: android/gradlew and the .githooks have no suffix.
  case "${file##*/}" in
    *.*) return 1 ;;
  esac
  if [ "$staged" = true ]; then
    first="$(git show ":$file" 2>/dev/null | head -n 1 || true)"
  else
    [ -f "$file" ] || return 1
    first="$(head -n 1 "$file" 2>/dev/null || true)"
  fi
  case "$first" in
    '#!'*bash*) return 0 ;;
    '#!'*'/sh'|'#!'*'/sh '*) return 0 ;;
    '#!'*'env sh'*) return 0 ;;
  esac
  return 1
}

files=()
if [ "$staged" = true ]; then
  while IFS= read -r file; do
    [ -n "$file" ] || continue
    is_shell_script "$file" && files+=("$file")
  done < <(git diff --cached --name-only --diff-filter=ACMR)
else
  while IFS= read -r file; do
    is_shell_script "$file" && files+=("$file")
  done < <(git ls-files)
fi

if [ "${#files[@]}" -eq 0 ]; then
  echo "shellcheck: no shell scripts to check"
  exit 0
fi

if ! command -v shellcheck >/dev/null 2>&1; then
  echo "shellcheck: not installed. Install it (apt install shellcheck, brew install shellcheck) and retry." >&2
  exit 1
fi

printf 'shellcheck: %d script(s)\n' "${#files[@]}"
shellcheck -x "${files[@]}"
