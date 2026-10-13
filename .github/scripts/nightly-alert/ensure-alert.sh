#!/usr/bin/env bash
# Shared core of the "nightly failure alarm" behavior: given one completed
# run of a scheduled workflow, either open/comment on that workflow's
# `nightly-failure`-labelled issue (on failure/timed_out/startup_failure/
# cancelled) or comment-and-close it (on success).
#
# Extracted from nightly-failure-alert.yml's own steps so the reconciliation
# sweep (nightly-failure-reconcile.yml — added after a Reference-Client E2E
# nightly failure never raised an alert because its `workflow_run` delivery
# was silently dropped, with no corresponding issue filed anywhere) can drive
# the exact same logic against a run it discovered by polling the Actions
# API, rather than forking a second copy of this bash that would drift from
# the first.
#
# Required env: GH_REPO, GH_TOKEN, WORKFLOW_NAME, RUN_ID, RUN_URL,
# CONCLUSION, RUN_ATTEMPT.
set -euo pipefail

: "${GH_REPO:?GH_REPO required}"
: "${GH_TOKEN:?GH_TOKEN required}"
: "${WORKFLOW_NAME:?WORKFLOW_NAME required}"
: "${RUN_ID:?RUN_ID required}"
: "${RUN_URL:?RUN_URL required}"
: "${CONCLUSION:?CONCLUSION required}"
: "${RUN_ATTEMPT:?RUN_ATTEMPT required}"
export GH_TOKEN GH_REPO

# shellcheck source=SCRIPTDIR/../alert-lib/issues.sh
source "$(dirname "${BASH_SOURCE[0]}")/../alert-lib/issues.sh"

echo "Nightly run of '${WORKFLOW_NAME}': ${CONCLUSION} (attempt ${RUN_ATTEMPT}) -- ${RUN_URL}"

gh label create nightly-failure \
	--color B60205 \
	--description "A scheduled (nightly/weekly/monthly) workflow run failed or has not yet recovered" \
	2>/dev/null || true

title="Nightly failure: ${WORKFLOW_NAME}"
# Exact-title lookup over the REST issues endpoint, never search/issues --
# see alert-lib/issues.sh for why (the Search API's 30/min budget killed
# the alarm on 2026-10-13).
existing_issue="$(find_alert_issue open "$title")"
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

	body="Scheduled run of **${WORKFLOW_NAME}** ended \`${CONCLUSION}\` (attempt ${RUN_ATTEMPT}): ${RUN_URL}

Failed/non-success job(s): ${failed}

Nightly is the zero-retry CI tier -- a flake here does not get an automatic re-run the way a PR check does. Investigate, or close this issue once triaged as a known flake; it reopens automatically (a fresh comment on the next failure) if it keeps happening, and this alert workflow closes it automatically the next time the scheduled run is green."
	if [ -n "$existing_issue" ]; then
		gh api --method POST "repos/${GH_REPO}/issues/${existing_issue}/comments" -f body="$body" >/dev/null
		echo "Commented on existing issue #${existing_issue}."
	else
		gh api --method POST "repos/${GH_REPO}/issues" \
			-f title="$title" \
			-f body="$body" \
			-f "labels[]=nightly-failure" >/dev/null
		echo "Opened a new nightly-failure issue for '${WORKFLOW_NAME}'."
	fi
	;;
success)
	if [ -z "$existing_issue" ]; then
		echo "No open nightly-failure issue for '${WORKFLOW_NAME}' -- nothing to close."
		exit 0
	fi
	body="Scheduled run of **${WORKFLOW_NAME}** succeeded: ${RUN_URL}

Closing -- the nightly run is green again."
	gh api --method POST "repos/${GH_REPO}/issues/${existing_issue}/comments" -f body="$body" >/dev/null
	gh api --method PATCH "repos/${GH_REPO}/issues/${existing_issue}" -f state=closed >/dev/null
	echo "Closed issue #${existing_issue}."
	;;
*)
	echo "Conclusion '${CONCLUSION}' is neither a failure state nor success -- nothing to do."
	;;
esac
