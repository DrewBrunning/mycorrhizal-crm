#!/usr/bin/env bash
# Issue #1905: fail when a Go module graph is untidy or its checksums do not
# match the module cache.
#
# Both modfiles are checked:
#   - go.mod is tidied in backend/, where its own packages are the roots;
#   - tools.mod is a separate graph holding only the CI `tool` directives
#     (issue #1551). A tidy run against backend/'s packages would copy every
#     main-module dependency into tools.mod, so it runs in a scratch module
#     directory where the tool directives are the only roots. go.mod is copied
#     there only because `-modfile` still needs a go.mod to locate the module
#     root; the Go toolchain does not read it.
#
# Usage:
#   check-go-modules.sh          # fail on drift (CI and pre-commit)
#   check-go-modules.sh --write  # rewrite go.mod/go.sum and tools.mod/tools.sum
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$repo_root/backend"

write=false
case "${1:-}" in
  "") ;;
  --write) write=true ;;
  *)
    echo "usage: $0 [--write]" >&2
    exit 2
    ;;
esac

if [ "$write" = true ]; then
  go mod tidy
else
  go mod tidy -diff
fi

scratch="$(mktemp -d)"
trap 'rm -rf "$scratch"' EXIT
cp go.mod tools.mod tools.sum "$scratch/"
if [ "$write" = true ]; then
  (cd "$scratch" && go mod tidy -modfile=tools.mod)
  cp "$scratch/tools.mod" tools.mod
  cp "$scratch/tools.sum" tools.sum
else
  (cd "$scratch" && go mod tidy -modfile=tools.mod -diff)
fi

go mod verify
go mod verify -modfile=tools.mod
