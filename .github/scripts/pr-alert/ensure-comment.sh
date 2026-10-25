#!/usr/bin/env bash
# Post (or update) one comment on a PR whose workflow run failed, naming the
# failing job, its log URL, and the local command that should have caught it
# (issue #1964).
#
# This is a convenience, not a gate: CI remains authoritative and agents must
# not poll `gh pr checks`. It runs from a `workflow_run` job checked out from
# the default branch, reads only the Actions API for the triggering run, and
# never checks out or executes PR code.
#
# Required env: GH_REPO, GH_TOKEN, WORKFLOW_NAME, RUN_ID, RUN_URL, CONCLUSION,
#               HEAD_SHA, RUN_ATTEMPT.
# Optional env: PR_NUMBER (resolved from HEAD_SHA when empty),
#               DRY_RUN=1 (log the intended comment without writing it).
set -euo pipefail

: "${GH_REPO:?GH_REPO required}"
: "${GH_TOKEN:?GH_TOKEN required}"
: "${WORKFLOW_NAME:?WORKFLOW_NAME required}"
: "${RUN_ID:?RUN_ID required}"
: "${RUN_URL:?RUN_URL required}"
: "${CONCLUSION:?CONCLUSION required}"
: "${HEAD_SHA:?HEAD_SHA required}"
: "${RUN_ATTEMPT:?RUN_ATTEMPT required}"
DRY_RUN="${DRY_RUN:-0}"
export GH_TOKEN GH_REPO

marker='<!-- pr-failure-alert -->'

pr_number="${PR_NUMBER:-}"
if [ -z "$pr_number" ]; then
	pr_number="$(gh api "repos/${GH_REPO}/commits/${HEAD_SHA}/pulls" \
		--jq '[.[] | select(.state == "open")][0].number // empty')"
fi
if [ -z "$pr_number" ]; then
	echo "No open PR found for ${HEAD_SHA}; nothing to comment on."
	exit 0
fi

# Map a failed job to the local command that covers it. Keep this in step with
# .github/filters.yaml / backend/internal/prepr/areas.go: the point is to name
# the command the contributor should have run, not a generic "see CI".
local_command() {
	local job="$1"
	case "$job" in
	Backend* | Frontend* | Android* | "Run E2E"* | "Docs & security-doc citations" | "Changelog note"*)
		echo "scripts/pre-pr.sh --full"
		;;
	"Container Hardening"* | "SAST"* | "Reproducible builds" | "GitHub Actions Security"*)
		echo "no local equivalent; this check is CI-only (see .github/AGENTS.md)"
		;;
	*)
		echo "scripts/pre-pr.sh --full"
		;;
	esac
}

jobs_json="$(gh api "repos/${GH_REPO}/actions/runs/${RUN_ID}/jobs?per_page=100")"
failed_jobs=""
while IFS=$'\t' read -r job_name job_url; do
	[ -z "$job_name" ] && continue
	failed_jobs+="- **${job_name}** — [job log](${job_url})"$'\n'"  - run locally: \`$(local_command "$job_name")\`"$'\n'
done < <(jq -r '
	[.jobs[] | select(.conclusion == "failure" or .conclusion == "timed_out")]
	| .[]
	| "\(.name)\t\(.html_url)"' <<<"$jobs_json")

if [ -z "$failed_jobs" ]; then
	failed_jobs="_No individual job reported a failure (workflow conclusion: ${CONCLUSION})._"
fi

body="$marker
## CI failed: ${WORKFLOW_NAME}

**Run:** [#${RUN_ID} attempt ${RUN_ATTEMPT}](${RUN_URL}) — \`${CONCLUSION}\`
**Commit:** \`${HEAD_SHA}\`

### Failing job(s)

${failed_jobs}

### Run locally

\`\`\`bash
scripts/pre-pr.sh --full
\`\`\`

The pre-PR gate selects the areas this branch touches and runs their suites,
coverage ratchets, and flake stress. Open the PR and stop; this comment is
updated in place on the next failure rather than duplicated."

existing="$(gh api "repos/${GH_REPO}/issues/${pr_number}/comments?per_page=100" --paginate \
	--jq ".[] | select(.body | contains(\"${marker}\")) | .id" | head -n1 || true)"

if [ "$DRY_RUN" = "1" ]; then
	echo "[dry-run] PR #${pr_number}: would ${existing:+update comment ${existing}}${existing:-create a comment} with:"
	printf '%s\n' "$body"
	exit 0
fi

if [ -n "$existing" ]; then
	gh api --method PATCH "repos/${GH_REPO}/issues/comments/${existing}" \
		--field body="$body" >/dev/null
	echo "Updated PR #${pr_number} comment ${existing}."
else
	gh api --method POST "repos/${GH_REPO}/issues/${pr_number}/comments" \
		--field body="$body" >/dev/null
	echo "Created a failure comment on PR #${pr_number}."
fi
