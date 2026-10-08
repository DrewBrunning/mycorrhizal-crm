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
# A `cancelled` conclusion is ambiguous (issue #1618): GitHub also marks a
# `push` run cancelled when a newer push run of the same workflow supersedes
# it -- push runs share a concurrency group, and a group's older *pending* run
# is cancelled when a newer one queues even without `cancel-in-progress`. That
# is not a failure: the newer run is coming to report the truth. Only a
# cancelled run with no newer run of the same workflow on the branch (cancelled
# by hand, or by the branch reaper) is a real alert.
#
# Required env: GH_REPO, GH_TOKEN, WORKFLOW_NAME, RUN_ID, RUN_URL,
# CONCLUSION, RUN_ATTEMPT, HEAD_SHA, HEAD_BRANCH.
#
# Optional env: DRY_RUN=1 makes every read-only lookup (the existing issue,
# the run's jobs, the originating PR, and the newer-run check) still run, but
# logs the issue/comment it *would* create, comment on or close instead of
# mutating anything. The `workflow_dispatch` test path in
# main-failure-alert.yml drives it this way by default, so the alarm can be
# exercised against a real run without touching the issue tracker.
#
# Optional env: NEWER_RUN_EXISTS=1|0 overrides the superseded-run lookup for
# the `workflow_dispatch` manual-test path, so both `cancelled` branches
# (superseded -> no action, lone -> alert) can be exercised without a live API
# coincidence. Unset in the event-driven path, where the lookup runs for real.
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

# run_was_superseded returns 0 when a newer push run of the same workflow on
# the same branch exists (so this run's cancellation is GitHub cancelling a
# superseded pending run), 1 otherwise -- including when the ordering cannot be
# established, which fails toward alerting rather than toward silence.
run_was_superseded() {
	# Manual-test override: let workflow_dispatch exercise both branches.
	if [ -n "${NEWER_RUN_EXISTS:-}" ]; then
		[ "$NEWER_RUN_EXISTS" = "1" ]
		return
	fi

	local this_json workflow_id this_number newer
	this_json="$(gh api "repos/${GH_REPO}/actions/runs/${RUN_ID}")"
	workflow_id="$(jq -r '.workflow_id // empty' <<<"$this_json")"
	this_number="$(jq -r '.run_number // empty' <<<"$this_json")"
	if [ -z "$workflow_id" ] || [ -z "$this_number" ]; then
		echo "Could not resolve the workflow id/run number for run ${RUN_ID} -- treating the cancellation as a real failure."
		return 1
	fi

	# run_number is monotonic within a workflow, so any higher number from a
	# newer push run on the same branch means this one was superseded.
	newer="$(gh api "repos/${GH_REPO}/actions/workflows/${workflow_id}/runs?branch=${HEAD_BRANCH}&event=push&per_page=5" \
		| jq --argjson n "$this_number" '[.workflow_runs[] | select((.run_number // 0) > $n)] | length')"
	[ "${newer:-0}" -gt 0 ]
}

case "$CONCLUSION" in
failure | timed_out | startup_failure | cancelled)
	label="$CONCLUSION"
	if [ "$CONCLUSION" = "cancelled" ]; then
		if run_was_superseded; then
			echo "Run of '${WORKFLOW_NAME}' on '${HEAD_BRANCH}' was cancelled but superseded by a newer push run -- no alert (the newer run reports the truth)."
			exit 0
		fi
		label="cancelled (not superseded)"
	fi

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

	body="Push run of **${WORKFLOW_NAME}** on \`${HEAD_BRANCH}\` ended \`${label}\` (attempt ${RUN_ATTEMPT}): ${RUN_URL}

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
