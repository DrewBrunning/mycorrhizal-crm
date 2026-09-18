#!/usr/bin/env bash
#
# Builds the per-gate `state` JSON array that the two mandatory-gate pollers
# (release.yml's "Gate - mandatory checks are green on the release commit" and
# docker-publish.yml's `release-gate` job) hand to release-gate-decide.sh.
#
# Extracted (issue #1150) so the classification of a gate that has no
# check-run *yet* cannot drift between the two pollers.
#
# The bug this exists to prevent: "Backend (Go)" is a `needs:`-gated fan-in job
# (it waits on backend-checks + every backend-tests matrix leg, then merges
# coverage). GitHub Actions does not create a job's check-run at all until its
# `needs:` have concluded, so while those legs are still running -- which on a
# busy `push: main` can easily outlast the poll deadline, dozens of workflows
# are queued at once -- the gate's context has no check-run and both pollers
# classified it as the literal "missing". release-gate-decide.sh treats
# "missing" as a structural absence (a hard block at the deadline, issue #913),
# which is right for a disabled/renamed/filter-regressed workflow but wrong
# here: the aggregation job has genuinely not run yet, which
# docs/development/release-gates.md already calls a timing signal, not absence.
# The gate's status becomes the owning workflow-run's status
# (`queued`/`in_progress`) when a run of that workflow is observably in flight
# on this commit, so the deadline yields WARN_PASS rather than FAIL_MISSING.
# A *completed* run with no check-run for the gate is left "missing" -- that is
# the real structural absence (#913), and this preserves it.
#
# Usage:
#   release-gate-state.sh <want_json> <since_json> <runs_file> \
#     <statuses_file> <workflow_runs_file> <gates_json>
#
#   want_json          JSON array of the required check-context names.
#   since_json         JSON object {context: ISO-timestamp}; a check-run or
#                      workflow-run produced before its context's timestamp is
#                      ignored (the #1013 stale-dispatch cutoff). An absent
#                      entry or "" means no cutoff.
#   runs_file          JSON array of check-run objects
#                      ({name, status, conclusion, started_at}).
#   statuses_file      JSON array of commit-status objects
#                      ({name, status, conclusion}).
#   workflow_runs_file JSON array of workflow-run objects
#                      ({path, status, conclusion, created_at}) -- from
#                      GET /repos/{owner}/{repo}/actions/runs?head_sha={sha}.
#   gates_json         path to .github/release-gates.json.
#
# Prints the state JSON array on stdout. Exits non-zero on malformed input.

set -euo pipefail

want_json="${1:?usage: release-gate-state.sh <want_json> <since_json> <runs_file> <statuses_file> <workflow_runs_file> <gates_json>}"
since_json="${2:?usage: release-gate-state.sh <want_json> <since_json> <runs_file> <statuses_file> <workflow_runs_file> <gates_json>}"
runs_file="${3:?usage: release-gate-state.sh <want_json> <since_json> <runs_file> <statuses_file> <workflow_runs_file> <gates_json>}"
statuses_file="${4:?usage: release-gate-state.sh <want_json> <since_json> <runs_file> <statuses_file> <workflow_runs_file> <gates_json>}"
workflow_runs_file="${5:?usage: release-gate-state.sh <want_json> <since_json> <runs_file> <statuses_file> <workflow_runs_file> <gates_json>}"
gates_json="${6:?usage: release-gate-state.sh <want_json> <since_json> <runs_file> <statuses_file> <workflow_runs_file> <gates_json>}"

jq -c -n \
  --argjson want "$want_json" \
  --argjson since "$since_json" \
  --slurpfile runs "$runs_file" \
  --slurpfile st "$statuses_file" \
  --slurpfile wf "$workflow_runs_file" \
  --slurpfile gates "$gates_json" '
  # check_context -> owning workflow file basename, from the registry.
  ($gates[0].gates
    | map(select(.release_gate == true))
    | map({(.check_context): (.workflow | split("/") | last)})
    | add // {}) as $ctx2wf
  # Latest observed check-run/commit-status per name, honouring the cutoff.
  | (($runs[0] + $st[0])
      | map(select((.started_at // "") >= ($since[.name] // "")))
      | group_by(.name)
      | map(max_by(.started_at // ""))
      | map({(.name): {status, conclusion}})
      | add // {}) as $latest
  # Latest workflow run per workflow basename, honouring the same cutoff.
  | ($wf[0]
      | map({wf: (.path | split("/") | last), status, conclusion, created_at}))
    as $wruns
  | [ $want[]
      | . as $name
      | ($ctx2wf[$name] // "") as $w
      | ($wruns
          | map(select(.wf == $w and (.created_at // "") >= ($since[$name] // "")))
          | sort_by(.created_at // "")
          | last) as $run
      | if $latest[$name] != null then
          {name: $name, status: $latest[$name].status, conclusion: ($latest[$name].conclusion // "")}
        elif $run != null and $run.status != "completed" then
          # No check-run yet, but the owning workflow is observably in flight:
          # a fan-in job whose dependencies have not finished. Timing, not
          # absence -- report the run so the deadline warns instead of blocks.
          {name: $name, status: $run.status, conclusion: ""}
        else
          {name: $name, status: "missing", conclusion: ""}
        end
    ]
'
