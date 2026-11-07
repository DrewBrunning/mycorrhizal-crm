#!/usr/bin/env bash
# Post the no-op codecov/patch/<area> success statuses for a PR whose own
# pull_request run could not (issue #1188 follow-up: fork and Dependabot PRs).
#
# Why this exists: Codecov only creates codecov/patch/* checks once it
# receives an upload. A PR whose changed paths trigger no coverage-uploading
# suite (docs-only, infra-only, codecov.yml itself) uploads nothing, so the
# three required contexts sit at "Expected" forever. unit-tests.yml's
# `codecov-patch-stub` job posts them for same-repo PRs, but a fork PR's
# GITHUB_TOKEN is read-only, so that job is skipped. This script runs from a
# `workflow_run` job (write token, default-branch code, no PR code executed)
# and does the same job for those PRs.
#
# Trust model: nothing the PR's own workflow run produced is believed. The
# PR is located from the run's head owner/branch (a fork run's
# `pull_requests` array is empty, and commits/<sha>/pulls returns nothing for
# a fork commit), its file list comes from the pulls API, and the verdict is
# computed by codecovstub from the DEFAULT BRANCH's .github/filters.yaml.
# Fail-safe everywhere: any doubt posts nothing, leaving the contexts for
# Codecov (real suites) or a human to resolve.
#
# Required env: GH_REPO, GH_TOKEN, HEAD_SHA, PATCH_AREAS, RUN_URL,
#               CODECOVSTUB_BIN (built from backend/cmd/codecovstub),
#               FILTERS_FILE (default-branch .github/filters.yaml).
# One of:       HEAD_OWNER + HEAD_BRANCH (the triggering run's head), or
#               PR_NUMBER (manual dispatch; HEAD_SHA is then read from the PR).
# Optional env: DRY_RUN=1 (log the statuses without posting them).
set -euo pipefail

: "${GH_REPO:?GH_REPO required}"
: "${GH_TOKEN:?GH_TOKEN required}"
: "${PATCH_AREAS:?PATCH_AREAS required}"
: "${RUN_URL:?RUN_URL required}"
: "${CODECOVSTUB_BIN:?CODECOVSTUB_BIN required}"
: "${FILTERS_FILE:?FILTERS_FILE required}"
DRY_RUN="${DRY_RUN:-0}"
export GH_TOKEN GH_REPO

pr_number="${PR_NUMBER:-}"
head_sha="${HEAD_SHA:-}"
if [ -z "$pr_number" ]; then
	: "${HEAD_SHA:?HEAD_SHA required}"
	: "${HEAD_OWNER:?HEAD_OWNER required}"
	: "${HEAD_BRANCH:?HEAD_BRANCH required}"
	pr_number="$(gh api -X GET "repos/${GH_REPO}/pulls" \
		-f state=open -f head="${HEAD_OWNER}:${HEAD_BRANCH}" \
		--jq "[.[] | select(.head.sha == \"${head_sha}\")][0].number // empty")"
	if [ -z "$pr_number" ]; then
		echo "No open PR has ${HEAD_OWNER}:${HEAD_BRANCH} at ${head_sha} (closed, or the branch moved on); nothing to post."
		exit 0
	fi
else
	head_sha="$(gh api "repos/${GH_REPO}/pulls/${pr_number}" --jq '.head.sha')"
fi

# Renames count under both names, like paths-filter. The pulls files API caps
# at 3000 files; a PR that large is never docs-only, so refuse to guess.
files="$(gh api --paginate "repos/${GH_REPO}/pulls/${pr_number}/files" \
	--jq '.[] | .filename, (.previous_filename // empty)')"
if [ "$(printf '%s\n' "$files" | grep -c .)" -ge 3000 ]; then
	echo "PR #${pr_number} lists >= 3000 files; not guessing, posting nothing."
	exit 0
fi

verdict="$(printf '%s\n' "$files" | "$CODECOVSTUB_BIN" "$FILTERS_FILE")"
echo "PR #${pr_number} @ ${head_sha}: codecovstub verdict: ${verdict}"
if [ "$verdict" != "no-upload" ]; then
	echo "A coverage-uploading suite runs for this PR; Codecov will report the statuses itself."
	exit 0
fi

for area in $PATCH_AREAS; do
	echo "codecov/patch/${area}: no coverage upload will happen for this commit; posting success (issue #1188)"
	if [ "$DRY_RUN" = "1" ]; then
		echo "DRY RUN: would POST success for codecov/patch/${area} on ${head_sha}"
		continue
	fi
	gh api --method POST "repos/${GH_REPO}/statuses/${head_sha}" \
		-f state=success \
		-f context="codecov/patch/${area}" \
		-f description="No coverage upload for this commit -- nothing to gate (issue #1188)" \
		-f target_url="${RUN_URL}" >/dev/null
done
