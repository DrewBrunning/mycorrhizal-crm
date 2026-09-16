#!/usr/bin/env bash
#
# Shared decision logic for the two mandatory-gate pollers (issue #913,
# REL-03 finding F3): docker-publish.yml's `release-gate` job and
# release.yml's "Gate - mandatory checks are green on the release commit"
# step. Both build the same `state` JSON (an array of
# {name, status, conclusion} per `release_gate: true` context, `status`
# forced to the literal string "missing" when no check-run/commit-status was
# ever found for that name) and used to decide independently what to do with
# it; this script is the one place that decision is made, so the two
# pollers cannot drift.
#
# Usage: release-gate-decide.sh <deadline_reached: true|false> < state.json
#
# Prints a decision on the first line of stdout, followed by the relevant
# gate name(s), one per line (empty if none). Always exits 0 -- the decision
# is communicated via stdout, not the exit code, so the caller decides what
# to do (poll again, warn-and-proceed, or fail the step).
#
#   PASS          -- every gate completed with an acceptable conclusion.
#   FAIL_OBSERVED -- at least one gate completed with an unacceptable
#                    conclusion (failure/cancelled/timed_out/action_required
#                    /etc). A hard block, checked BEFORE the deadline --
#                    this has always been true regardless of what else is
#                    still pending.
#   POLL          -- some gates are unfinished (including "missing") but the
#                    deadline has not been reached yet. Keep waiting. A
#                    freshly-dispatched RC gate workflow is expected to read
#                    back "missing" for a while before its first check-run
#                    appears -- that is not evidence of anything wrong, so
#                    this must never be treated as a failure pre-deadline.
#   FAIL_MISSING  -- the deadline was reached and at least one gate's status
#                    is still literally "missing": no check-run or commit
#                    status for it has EVER been observed on this commit.
#                    That is a structural absence (a disabled/renamed
#                    workflow, a filter regression that skips the job
#                    entirely, a failed dispatch), not a slow check, and
#                    issue #913's finding is exactly that treating it as
#                    "no failure observed" and publishing anyway is silently
#                    equivalent to "verified". This is a hard block, same as
#                    FAIL_OBSERVED -- the only way past it is the existing
#                    workflow_dispatch + override_reason escape hatch.
#   WARN_PASS     -- the deadline was reached, nothing is missing, but one
#                    or more gates that DID start are still running. This is
#                    the one case that publishes with only a warning -- a
#                    gate that is observably in flight (queued/in_progress)
#                    is a timing question, not a correctness one.

set -u

deadline_reached="${1:?usage: release-gate-decide.sh <deadline_reached: true|false>}"
state="$(cat)"

failed="$(printf '%s' "$state" | jq -r '
  .[] | select(.status == "completed" and .conclusion != "" and ([.conclusion] | inside(["success","neutral","skipped"]) | not)) | .name')"
if [ -n "$failed" ]; then
  echo "FAIL_OBSERVED"
  printf '%s\n' "$failed"
  exit 0
fi

unfinished="$(printf '%s' "$state" | jq -r '.[] | select(.status != "completed" or .conclusion == "") | .name')"
if [ -z "$unfinished" ]; then
  echo "PASS"
  exit 0
fi

if [ "$deadline_reached" != "true" ]; then
  echo "POLL"
  printf '%s\n' "$unfinished"
  exit 0
fi

missing="$(printf '%s' "$state" | jq -r '.[] | select(.status == "missing") | .name')"
if [ -n "$missing" ]; then
  echo "FAIL_MISSING"
  printf '%s\n' "$missing"
  exit 0
fi

echo "WARN_PASS"
printf '%s\n' "$unfinished"
