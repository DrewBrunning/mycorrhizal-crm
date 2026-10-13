#!/usr/bin/env bash
# Issue #1661: given the verdict JSON of `go run ./cmd/bypassedcheck` for a
# merged PR's head SHA, open (or comment on) a `ci`+`p1` issue
# "Merged with non-green required checks: PR #N" when any required context was
# not green. Silent when the verdict has no findings.
#
# Same upsert shape as .github/scripts/main-alert/ensure-alert.sh: REST search
# (`gh issue list` is unreliable on this repo), one issue per PR, a re-run
# comments instead of duplicating.
#
# Required env: GH_REPO, GH_TOKEN, PR_NUMBER, VERDICT_FILE.
# Optional env: DRY_RUN=1 logs the issue/comment instead of writing it.
set -euo pipefail

: "${GH_REPO:?GH_REPO required}"
: "${GH_TOKEN:?GH_TOKEN required}"
: "${PR_NUMBER:?PR_NUMBER required}"
: "${VERDICT_FILE:?VERDICT_FILE required}"
DRY_RUN="${DRY_RUN:-0}"
export GH_TOKEN GH_REPO

count="$(jq '.findings | length' "$VERDICT_FILE")"
if [ "$count" -eq 0 ]; then
	echo "PR #${PR_NUMBER}: every required check was green -- no alert."
	exit 0
fi

pr_json="$(gh api "repos/${GH_REPO}/pulls/${PR_NUMBER}")"
pr_title="$(jq -r '.title' <<<"$pr_json")"
pr_url="$(jq -r '.html_url' <<<"$pr_json")"
merger="$(jq -r '.merged_by.login // "unknown"' <<<"$pr_json")"
head_sha="$(jq -r '.sha' "$VERDICT_FILE")"

lines="$(jq -r '.findings[] | "- `\(.context)` -- \(.category) (\(.detail))"' "$VERDICT_FILE")"

title="Merged with non-green required checks: PR #${PR_NUMBER}"
body="${pr_url} (\`${pr_title}\`) was merged by @${merger} while ${count} required check(s) were not green on its head commit \`${head_sha}\`:

${lines}

Categories: \`failed\` = the latest result was a non-passing conclusion (a cancelled run counts: the branch reaper cancels still-running jobs at merge); \`incomplete\` = the PR was merged before that required check finished; \`missing\` = no check run or status was ever reported for the context.

This is the 'gates are not silently weakened' case: the merge bypassed (or raced) a required check. Check the post-merge \`push\` run on \`main\` and fix forward -- do not just lower a baseline. Close this issue once the underlying failure is resolved or the bypass is explained here. See docs/development/repo-governance.md (merge-bypass alert)."

if [ "$DRY_RUN" = "1" ]; then
	echo "[dry-run] would ensure labels ci,p1 exist and upsert issue '${title}' with body:"
	printf '%s\n' "$body"
	exit 0
fi

gh label create ci --color 0e8a16 --description "CI / workflow / gate infrastructure" 2>/dev/null || true
gh label create p1 --color fbca04 --description "Should be scheduled - belongs in an upcoming milestone" 2>/dev/null || true

query="repo:${GH_REPO} is:issue is:open in:title \"${title}\""
encoded="$(jq -rn --arg q "$query" '$q | @uri')"
existing="$(gh api -X GET "search/issues?q=${encoded}" --jq '.items[0].number // empty')"
if [ -n "$existing" ]; then
	gh api --method POST "repos/${GH_REPO}/issues/${existing}/comments" -f body="$body" >/dev/null
	echo "Commented on existing issue #${existing}."
else
	gh api --method POST "repos/${GH_REPO}/issues" \
		-f title="$title" -f body="$body" -f "labels[]=ci" -f "labels[]=p1" >/dev/null
	echo "Opened an alert issue for PR #${PR_NUMBER}."
fi
