#!/usr/bin/env bash
# Issue #1661: given the verdict JSON of `go run ./cmd/bypassedcheck` for a
# merged PR's head SHA, open (or comment on) a `ci`+`p1` issue
# "Merged with non-green required checks: PR #N" when a required context ended
# up not green. Silent (job summary only) otherwise.
#
# The verdict is already settled: cmd/bypassedcheck polled checks that were
# still queued/running at merge time until they finished or its -wait-max
# ran out, so a finding here is one of
#   failed      the check finished red (or cancelled);
#   missing     the check never reported at all;
#   incomplete  the check was STILL unfinished after the whole wait.
# A check that was unfinished at merge but later finished green
# (`merged_early` minus findings) is not an alert -- it is recorded in the job
# summary, so the merged-before-finished fact stays visible. Rationale in
# docs/development/repo-governance.md (merge-bypass alert).
#
# Same upsert shape as .github/scripts/main-alert/ensure-alert.sh: one issue
# per PR, found by exact title through the REST issues endpoint (never the
# Search API -- see alert-lib/issues.sh), a re-run comments instead of
# duplicating.
#
# Required env: GH_REPO, GH_TOKEN, PR_NUMBER, VERDICT_FILE.
# Optional env: DRY_RUN=1 logs the issue/comment instead of writing it;
# GITHUB_STEP_SUMMARY (appended to when set).
set -euo pipefail

: "${GH_REPO:?GH_REPO required}"
: "${GH_TOKEN:?GH_TOKEN required}"
: "${PR_NUMBER:?PR_NUMBER required}"
: "${VERDICT_FILE:?VERDICT_FILE required}"
DRY_RUN="${DRY_RUN:-0}"
export GH_TOKEN GH_REPO

# shellcheck source=SCRIPTDIR/../alert-lib/issues.sh
source "$(dirname "${BASH_SOURCE[0]}")/../alert-lib/issues.sh"

summary() {
	echo "$1"
	if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
		printf '%s\n' "$1" >>"$GITHUB_STEP_SUMMARY"
	fi
}

count="$(jq '.findings | length' "$VERDICT_FILE")"
waited_min="$(jq -r '(.waited_seconds // 0) / 60 | floor' "$VERDICT_FILE")"

# Contexts that were unfinished when the PR merged and later finished green.
late_green="$(jq -r '(.findings | map(.context)) as $f | (.merged_early // []) | map(select(. as $c | $f | index($c) | not)) | map("`" + . + "`") | join(", ")' "$VERDICT_FILE")"

if [ "$count" -eq 0 ]; then
	if [ -n "$late_green" ]; then
		summary "PR #${PR_NUMBER}: merged before ${late_green} finished; it later finished green (waited ${waited_min} min) -- no alert."
	else
		summary "PR #${PR_NUMBER}: every required check was green -- no alert."
	fi
	exit 0
fi

pr_json="$(gh api "repos/${GH_REPO}/pulls/${PR_NUMBER}")"
pr_title="$(jq -r '.title' <<<"$pr_json")"
pr_url="$(jq -r '.html_url' <<<"$pr_json")"
merger="$(jq -r '.merged_by.login // "unknown"' <<<"$pr_json")"
head_sha="$(jq -r '.sha' "$VERDICT_FILE")"

lines="$(jq -r --argjson w "$waited_min" '
	(.merged_early // []) as $early
	| .findings[]
	| "- `\(.context)` -- " +
	  (if .category == "incomplete" then "merged before it finished and still incomplete (\(.detail)) after waiting \($w) min"
	   elif (.context as $c | $early | index($c)) then "merged before it finished; it later finished red (\(.detail))"
	   elif .category == "failed" then "failed (\(.detail))"
	   else "never reported (\(.detail))" end)' "$VERDICT_FILE")"

title="Merged with non-green required checks: PR #${PR_NUMBER}"
body="${pr_url} (\`${pr_title}\`) was merged by @${merger} while ${count} required check(s) were not green on its head commit \`${head_sha}\`:

${lines}

Checks still running at merge time were re-evaluated for ${waited_min} min before this alert. \`failed\` = the latest result is a non-passing conclusion (a cancelled run counts: the branch reaper cancels still-running jobs at merge); \`missing\` = no check run or status was ever reported for the context.

This is the 'gates are not silently weakened' case: the merge bypassed (or raced) a required check. Check the post-merge \`push\` run on \`main\` and fix forward -- do not just lower a baseline. Close this issue once the underlying failure is resolved or the bypass is explained here. See docs/development/repo-governance.md (merge-bypass alert)."

if [ "$DRY_RUN" = "1" ]; then
	echo "[dry-run] would ensure labels ci,p1 exist and upsert issue '${title}' with body:"
	printf '%s\n' "$body"
	exit 0
fi

gh label create ci --color 0e8a16 --description "CI / workflow / gate infrastructure" 2>/dev/null || true
gh label create p1 --color fbca04 --description "Should be scheduled - belongs in an upcoming milestone" 2>/dev/null || true

existing="$(find_labeled_issue ci open "$title")"
if [ -n "$existing" ]; then
	gh api --method POST "repos/${GH_REPO}/issues/${existing}/comments" -f body="$body" >/dev/null
	echo "Commented on existing issue #${existing}."
else
	gh api --method POST "repos/${GH_REPO}/issues" \
		-f title="$title" -f body="$body" -f "labels[]=ci" -f "labels[]=p1" >/dev/null
	echo "Opened an alert issue for PR #${PR_NUMBER}."
fi
