#!/usr/bin/env bash
#
# Watch a workflow run, retrying only when GitHub's Actions API returns a
# transient 429/5xx while polling (issue #2900).
#
# The release dry-run rehearsal dispatches release.yml and then waits with
# `gh run watch --exit-status`. On 2026-11-23 that poll died with
# "failed to get jobs: HTTP 500 (https://api.github.com/.../jobs)" and failed
# the nightly even though the watched run was healthy. `gh run watch
# --exit-status` uses one non-zero status for two different outcomes -- the
# watched run failed, or a poll hit an API error -- so retrying blindly could
# hide a real rehearsal failure.
#
# This wrapper retries ONLY when the captured output carries an HTTP 429/5xx
# signature. A non-successful watched run (no such signature) exits with
# gh's own status on the first attempt.
#
# Usage: gh-run-watch-retry.sh <run-id>
# Env: GH_RUN_WATCH_MAX_ATTEMPTS (default 3 -- two retries),
#      GH_RUN_WATCH_DELAY_SECONDS (default 20 -- doubles per retry, capped at 120).
set -uo pipefail

run_id="${1:?usage: gh-run-watch-retry.sh <run-id>}"

attempts="${GH_RUN_WATCH_MAX_ATTEMPTS:-3}"
case "$attempts" in
'' | *[!0-9]*)
	echo "::error::GH_RUN_WATCH_MAX_ATTEMPTS must be a positive integer (got '$attempts')" >&2
	exit 2
	;;
esac
if [ "$attempts" -lt 1 ]; then
	echo "::error::GH_RUN_WATCH_MAX_ATTEMPTS must be >= 1 (got '$attempts')" >&2
	exit 2
fi

delay="${GH_RUN_WATCH_DELAY_SECONDS:-20}"
case "$delay" in
'' | *[!0-9]*)
	echo "::error::GH_RUN_WATCH_DELAY_SECONDS must be a non-negative integer (got '$delay')" >&2
	exit 2
	;;
esac

# Deliberately narrow: an Actions API transport failure, never a failed run.
transient_re='HTTP (429|5[0-9][0-9])'

out="$(mktemp)"
trap 'rm -f "$out"' EXIT

attempt=1
while :; do
	gh run watch "$run_id" --exit-status 2>&1 | tee "$out"
	rc=${PIPESTATUS[0]}
	if [ "$rc" -eq 0 ]; then
		if [ "$attempt" -gt 1 ]; then
			echo "::warning::FLAKE: gh run watch passed on retry after a transient Actions API error (issue #2900). Triage; the check is not blocked." >&2
		fi
		exit 0
	fi
	# A non-successful watched run carries no API signature; it is the real
	# answer and must fail on the first attempt.
	if ! grep -qiE "$transient_re" "$out"; then
		exit "$rc"
	fi
	if [ "$attempt" -ge "$attempts" ]; then
		echo "::error::gh run watch failed again after a transient Actions API error -- the API is degraded, not a one-off flake (issue #2900)" >&2
		exit "$rc"
	fi
	attempt=$((attempt + 1))
	backoff=$((delay * (1 << (attempt - 2))))
	[ "$backoff" -gt 120 ] && backoff=120
	echo "::warning::gh run watch hit a transient Actions API error -- waiting ${backoff}s before retrying (attempt ${attempt}/${attempts}, issue #2900)" >&2
	grep -iE "$transient_re" "$out" | head -3 >&2 || true
	[ "$backoff" -gt 0 ] && sleep "$backoff"
done
