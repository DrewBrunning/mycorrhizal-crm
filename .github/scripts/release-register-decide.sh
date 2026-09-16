#!/usr/bin/env bash
#
# Decides whether release.yml's "Register the release" step should append a
# new backend/internal/schemafixture.SupportedReleases entry, or skip because
# the version is already registered (issue #929).
#
# Bug this exists to prevent: `dry_run: true` is documented as the way to
# rehearse the whole final-release path, including against the last shipped
# version (step 1's "already registered" check is downgraded to a warning for
# exactly that case) -- but the registration step itself had no matching
# guard, so a dry run targeting an already-shipped version unconditionally
# appended a SECOND {Tag: "<version>", ...} entry for it. `cmd/genschema` then
# rewrote that version's dump byte-identically (there was nothing new to
# generate), so the downstream "Assert only the new dump changed" step's
# `git status --porcelain` found NO diff where it expected exactly one new
# untracked file, and failed every time -- the one documented rehearsal of the
# final-release path could never pass.
#
# Usage: release-register-decide.sh <releases.go path> <version>
#
# Prints REGISTER (the version is not yet in SupportedReleases -- append it,
# and the dump-regeneration step downstream must expect exactly one new
# untracked dump) or SKIP (it is already there -- do not re-add it, and the
# dump-regeneration step downstream must expect NO diff at all) on stdout.
# Exit 0 always; the decision is communicated via stdout, not the exit code.

set -u

path="${1:?usage: release-register-decide.sh <releases.go path> <version>}"
version="${2:?usage: release-register-decide.sh <releases.go path> <version>}"

# Matches the exact `{Tag: "$version",` form the registration step writes
# (internal/schemafixture/releases.go, promote-rc.yml's mirror of the same
# append) -- anchored on the trailing comma so "v0.8" cannot false-positive
# against an already-registered "v0.8.3".
if grep -qF "{Tag: \"${version}\"," "$path"; then
  echo "SKIP"
else
  echo "REGISTER"
fi
