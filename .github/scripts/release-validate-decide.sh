#!/usr/bin/env bash
#
# Decides how release.yml's "Validate version input" step should treat the
# requested version (issue #1142): refuse, resume, or proceed.
#
# Bug this exists to prevent: a final release can reach step 8 (the
# release-tier suite wait) and fail *after* step 7 has already committed and
# pushed the schema-fixture registration to main. The version is now in
# SupportedReleases but has no tag -- a partially cut release. The old step 1
# hard-refused any already-registered version, so the documented recovery
# ("fix on main and re-dispatch") was impossible: the re-dispatch died at
# validation before it could resume. This decision point makes the workflow
# re-entrant for exactly that window, and *only* that window:
#
#   - tag already exists (real run)  -> FAIL_TAG_EXISTS. A released tag is
#     never moved; the recovery for a post-tag docker-publish failure is
#     docker-publish.yml's own `tag` input, not a second release run.
#   - registered, untagged, final    -> RESUME. The fixture commit is already
#     on main; the caller resumes at the checked-out tip (which now carries
#     whatever fix unblocked the earlier attempt), re-running the gates and
#     release-tier suites before pushing the tag.
#   - everything else                -> PROCEED (a fresh version, or a dry run,
#     where the tag/registered checks are downgraded to warnings).
#
# Usage: release-validate-decide.sh <is_rc> <dry_run> <tag_exists> <registered>
#   is_rc       "true" when the version carries an -rc.N suffix
#   dry_run     the workflow dispatch's dry_run input
#   tag_exists  "true" when refs/tags/<version> already exists
#   registered  "true" when <version> is already in SupportedReleases
#
# Prints exactly one of FAIL_TAG_EXISTS / RESUME / PROCEED on stdout. Exit 0
# always; the decision is communicated via stdout, not the exit code.

set -u

is_rc="${1:?usage: release-validate-decide.sh <is_rc> <dry_run> <tag_exists> <registered>}"
dry_run="${2:?usage: release-validate-decide.sh <is_rc> <dry_run> <tag_exists> <registered>}"
tag_exists="${3:?usage: release-validate-decide.sh <is_rc> <dry_run> <tag_exists> <registered>}"
registered="${4:?usage: release-validate-decide.sh <is_rc> <dry_run> <tag_exists> <registered>}"

if [ "$tag_exists" = "true" ] && [ "$dry_run" != "true" ]; then
  echo "FAIL_TAG_EXISTS"
  exit 0
fi

if [ "$is_rc" != "true" ] && [ "$dry_run" != "true" ] && [ "$tag_exists" != "true" ] && [ "$registered" = "true" ]; then
  echo "RESUME"
  exit 0
fi

echo "PROCEED"
