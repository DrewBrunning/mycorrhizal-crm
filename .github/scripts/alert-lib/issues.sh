# shellcheck shell=bash
# Issue lookups shared by the nightly/main failure alarms
# (nightly-alert/ensure-alert.sh, nightly-alert/already-reported.sh,
# main-alert/ensure-alert.sh). Sourced, not executed; requires GH_REPO and an
# authenticated `gh`.
#
# Deliberately the REST issues endpoint, never `search/issues`. The Search
# API has its own budget of 30 requests/minute per installation, separate
# from the core budget, and nightly-failure-reconcile.yml fans out ~33
# matrix legs inside the same couple of seconds: on 2026-10-13 every leg
# past the 30th died "API rate limit exceeded for installation" (run
# 38400077165), and the nightly-failure-alert.yml run reporting *that*
# failure died the same way a few seconds later (run 38400127969), so the
# alarm silently alerted no one. Search is also eventually consistent (a
# just-opened issue may not be indexed yet, so a second failure moments
# later could open a duplicate) and `in:title` is a phrase match, not an
# equality test; here the title is compared exactly.
#
# `gh issue list` is not an option either: it is unreliable against this
# repo (Projects-classic GraphQL error).

# labeled_issues LABEL STATE -> JSON array of every LABEL-labelled issue
# (pull requests excluded) in STATE (open|closed|all), newest first.
labeled_issues() {
	gh api --paginate "repos/${GH_REPO}/issues?labels=$1&state=$2&sort=created&direction=desc&per_page=100" |
		jq -s '[.[][] | select(.pull_request | not)]'
}

# find_labeled_issue LABEL STATE TITLE -> the number of the newest LABEL
# issue in STATE whose title is exactly TITLE, or nothing.
find_labeled_issue() {
	local issues
	issues="$(labeled_issues "$1" "$2")"
	jq -r --arg t "$3" '[.[] | select(.title == $t)][0].number // empty' <<<"$issues"
}

# alert_issues STATE -> JSON array of every `nightly-failure`-labelled issue
# (pull requests excluded) in STATE (open|closed|all), newest first.
alert_issues() {
	labeled_issues nightly-failure "$1"
}

# find_alert_issue STATE TITLE -> the number of the newest issue in STATE
# whose title is exactly TITLE, or nothing.
find_alert_issue() {
	local issues
	issues="$(alert_issues "$1")"
	jq -r --arg t "$2" '[.[] | select(.title == $t)][0].number // empty' <<<"$issues"
}

# alert_run_reported TITLE RUN_ID -> exit 0 when the newest issue titled
# exactly TITLE (open or closed) already links run RUN_ID in its body or in
# any comment, 1 otherwise. A run is reported to the issue for its own
# workflow's title, and a newer issue with that title only exists if a later
# run opened it, so the newest one is the only place to look.
alert_run_reported() {
	local title="$1" run_id="$2" number texts
	number="$(find_alert_issue all "$title")"
	[ -n "$number" ] || return 1
	# Captured, not piped into `grep -q`: grep exiting at the first match
	# SIGPIPEs gh, and under the callers' `pipefail` that turns a hit into
	# a miss.
	texts="$(gh api "repos/${GH_REPO}/issues/${number}" --jq '.body // ""')"
	texts+=$'\n'"$(gh api --paginate "repos/${GH_REPO}/issues/${number}/comments?per_page=100" --jq '.[].body // ""')"
	# Anchor on the run URL's `/runs/<id>` segment so run 12345 is not
	# "found" inside run 123456's link.
	grep -Eq -- "/runs/${run_id}([^0-9]|$)" <<<"$texts"
}
