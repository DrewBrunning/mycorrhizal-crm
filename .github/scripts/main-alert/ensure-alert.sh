#!/usr/bin/env bash
# Shared core of the "red main" alarm (issue #1568): given one completed
# `push` run of a gate-carrying workflow on main, either open/comment on that
# workflow's issue (on failure/timed_out/startup_failure/cancelled) or
# comment-and-close it (on success).
#
# Modelled on .github/scripts/nightly-alert/ensure-alert.sh, but kept separate
# because the title prefix (`Main failure:`, not `Nightly failure:`), the
# labels (`nightly-failure` + `p1`) and the body (head SHA + originating PR)
# are main-push-specific; the nightly script's issue matching must stay scoped
# to `Nightly failure:` titles so the two alarms can never close each other's
# issues.
#
# Required env: GH_REPO, GH_TOKEN, WORKFLOW_NAME, RUN_ID, RUN_URL,
# CONCLUSION, RUN_ATTEMPT, HEAD_SHA, HEAD_BRANCH.
#
# Optional env: DRY_RUN=1 makes every read-only lookup (the existing issue,
# the run's jobs, the originating PR) still run, but logs the issue/comment it
# *would* create, comment on or close instead of mutating anything. The
# `workflow_dispatch` test path in main-failure-alert.yml drives it this way by
# default, so the alarm can be exercised against a real run without touching
# the issue tracker.
set -euo pipefail

: "${GH_REPO:?GH_REPO required}"
: "${GH_TOKEN:?GH_TOKEN required}"
: "${WORKFLOW_NAME:?WORKFLOW_NAME required}"
: "${RUN_ID:?RUN_ID required}"
: "${RUN_URL:?RUN_URL required}"
: "${CONCLUSION:?CONCLUSION required}"
: "${RUN_ATTEMPT:?RUN_ATTEMPT required}"
: "${HEAD_SHA:?HEAD_SHA required}"
: "${HEAD_BRANCH:?HEAD_BRANCH required}"
DRY_RUN="${DRY_RUN:-0}"
export GH_TOKEN GH_REPO

echo "Main run of '${WORKFLOW_NAME}': ${CONCLUSION} (attempt ${RUN_ATTEMPT}) -- ${RUN_URL}"

if [ "$DRY_RUN" = "1" ]; then
	echo "[dry-run] would ensure the 'nightly-failure' and 'p1' labels exist."
else
	gh label create nightly-failure \
		--color B60205 \
		--description "A scheduled (nightly/weekly/monthly) workflow run failed or has not yet recovered" \
		2>/dev/null || true
	gh label create p1 \
		--color fbca04 \
		--description "Should be scheduled - belongs in an upcoming milestone" \
		2>/dev/null || true
fi

title="Main failure: ${WORKFLOW_NAME}"
# `gh issue list` is unreliable against this repo (Projects-classic GraphQL
# error) -- go through the REST search API directly, the same workaround this
# repo's other automation uses. The query string is built with jq's @uri so
# the title's own characters (parens, etc. in a workflow name like "Chaos
# Tests (failure injection)") cannot break out of the quoted in:title term.
query="repo:${GH_REPO} is:issue is:open label:nightly-failure in:title \"${title}\""
encoded="$(jq -rn --arg q "$query" '$q | @uri')"
existing_issue="$(gh api -X GET "search/issues?q=${encoded}" --jq '.items[0].number // empty')"
if [ -n "$existing_issue" ]; then
	echo "Found existing open issue #${existing_issue} for '${WORKFLOW_NAME}'."
else
	echo "No existing open issue for '${WORKFLOW_NAME}'."
fi

case "$CONCLUSION" in
failure | timed_out | startup_failure | cancelled)
	failed="$(gh api "repos/${GH_REPO}/actions/runs/${RUN_ID}/jobs" --paginate \
		--jq '[.jobs[] | select(.conclusion != "success" and .conclusion != "skipped" and .conclusion != null) | .name] | join(", ")')"
	if [ -z "$failed" ]; then
		failed="(no individual job reported a non-success conclusion -- see the run for details)"
	fi

	# The commit a push run came from almost always belongs to a merged PR,
	# but a direct push (or a release-registration commit) has none -- handle
	# the empty case explicitly rather than printing a dangling "#".
	pr="$(gh api "repos/${GH_REPO}/commits/${HEAD_SHA}/pulls" --jq '.[0].number // empty')"
	if [ -n "$pr" ]; then
		origin="the pull request it came from: #${pr}"
	else
		origin="no associated pull request (direct push to \`${HEAD_BRANCH}\`)"
	fi

	body="Push run of **${WORKFLOW_NAME}** on \`${HEAD_BRANCH}\` ended \`${CONCLUSION}\` (attempt ${RUN_ATTEMPT}): ${RUN_URL}

Head commit: \`${HEAD_SHA}\` -- ${origin}

Failed/non-success job(s): ${failed}

This is the post-merge verification of the change. If the PR was merged before its required checks finished -- or via an admin bypass -- this run is the only full verification the change got, because the reaper cancels the branch's still-running required jobs at merge. Investigate; this alert closes itself automatically the next time this workflow is green on \`main\` (a fresh comment appears if it fails again while the issue is still open)."
	if [ "$DRY_RUN" = "1" ]; then
		if [ -n "$existing_issue" ]; then
			echo "[dry-run] would comment on issue #${existing_issue} with:"
		else
			echo "[dry-run] would open a new issue titled '${title}' with labels nightly-failure,p1 and body:"
		fi
		printf '%s\n' "$body"
		exit 0
	fi
	if [ -n "$existing_issue" ]; then
		gh api --method POST "repos/${GH_REPO}/issues/${existing_issue}/comments" -f body="$body" >/dev/null
		echo "Commented on existing issue #${existing_issue}."
	else
		gh api --method POST "repos/${GH_REPO}/issues" \
			-f title="$title" \
			-f body="$body" \
			-f "labels[]=nightly-failure" \
			-f "labels[]=p1" >/dev/null
		echo "Opened a new main-failure issue for '${WORKFLOW_NAME}'."
	fi
	;;
success)
	if [ -z "$existing_issue" ]; then
		echo "No open main-failure issue for '${WORKFLOW_NAME}' -- nothing to close."
		exit 0
	fi
	body="Push run of **${WORKFLOW_NAME}** on \`${HEAD_BRANCH}\` succeeded: ${RUN_URL}

Closing -- \`main\` is green again for this workflow."
	if [ "$DRY_RUN" = "1" ]; then
		echo "[dry-run] would comment on and close issue #${existing_issue} with:"
		printf '%s\n' "$body"
		exit 0
	fi
	gh api --method POST "repos/${GH_REPO}/issues/${existing_issue}/comments" -f body="$body" >/dev/null
	gh api --method PATCH "repos/${GH_REPO}/issues/${existing_issue}" -f state=closed >/dev/null
	echo "Closed issue #${existing_issue}."
	;;
*)
	echo "Conclusion '${CONCLUSION}' is neither a failure state nor success -- nothing to do."
	;;
esac
