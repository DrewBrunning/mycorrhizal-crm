# Agent evaluation tasks

`cmd/agenteval` runs one representative task per registered suite in a
disposable worktree and scores the result. It is dispatch-only (or weekly via
`agent-eval-collect.yml`) and never a merge or release gate. See
`docs/development/context-engineering.md`.

## Pinned profiles

`agents.yaml` holds the pinned agent configurations. A profile names the
command, model and CLI version, and a real profile's command must contain
`{prompt}`. Use it for a comparable run:

```bash
cd backend && go run ./cmd/agenteval -task database-migration -profile pinned-opencode
```

The same profile pins trigger-selection runs:

```bash
cd backend && go run ./cmd/agenteval -mode select -profile pinned-opencode
```

The promoted profile is `pinned-opencode`: opencode CLI `1.19.4`, model
`deepseek/deepseek-flash`. Its command installs that exact CLI with `npx`, maps
`AGENT_EVAL_MODEL_KEY` to the provider variable opencode reads, and runs with
`--pure --auto` so the collector needs no preinstalled agent. Set
`AGENT_EVAL_MODEL_KEY` for a local run with the same profile. Its
`version_command` is run before the agent: the observed CLI version is recorded
in the artifact and a mismatch with the pinned `version` fails the run before a
model call.

Explicit `-agent-command`/`-agent-model`/`-agent-version` flags cannot be
combined with `-profile`; a free-form run is not comparable with a baseline.
`dry-run` profiles are for smoke tests only and are rejected when a baseline is
involved or when selection needs a command that writes a report.

## Baseline

`baseline.json` records the last accepted run per profile and task: success,
suite commands passed/failed, scope violations, generated-artifact hand-edits,
gate-integrity violations, content-check failures, refusal-task unexpected
changes and a fingerprint of the agent, prompt, base ref and suite. A missing
entry is "no baseline", never a regression by itself.

Tasks may also declare `content_checks` in `spec.yaml`, asserting that the
changed files contain the behavior the prompt asked for (for example naming the
regeneration command, or carrying a plural key in every locale). Suite commands
prove a gate still exits 0; content checks prove the requested change exists.

Compare a run against it:

```bash
cd backend && go run ./cmd/agenteval -task database-migration -profile pinned \
  -baseline testdata/agent-eval/baseline.json -no-control
```

Pass `-require-baseline` to fail when the profile/task has no entry (the
collector does). Promote a known-good run and commit the diff:

```bash
cd backend && go run ./cmd/agenteval -task database-migration -profile pinned-opencode \
  -write-baseline testdata/agent-eval/baseline.json -no-control
```

When the run happened in CI and the model credential is not local, download its
artifact and promote the measured result instead of editing the baseline:

```bash
cd backend && go run ./cmd/agenteval -promote-artifact /path/to/artifact.json \
  -write-baseline testdata/agent-eval/baseline.json \
  -promotion-source https://github.com/DrewBrunning/mycorrhizal-crm/actions/runs/ID
```

Promotion refuses an artifact that is not a passing run of the committed
profile, prompt, base ref and content checks. Only `-write-baseline` and
`-promote-artifact` set `verified`; a hand-edited `config_hash` is unverified
even when its recomputation matches, and a comparison prints that warning.

Refresh every task after a profile, prompt, spec or suite change (each passing
run merges into the same file; failing tasks are refused and need review):

```bash
cd backend
for task in $(find testdata/agent-eval -mindepth 2 -maxdepth 2 -name spec.yaml -printf '%h\n' | sed 's#testdata/agent-eval/##' | sort); do
  go run ./cmd/agenteval -task "$task" -profile pinned-opencode \
    -write-baseline testdata/agent-eval/baseline.json -no-control || break
done
```

A non-passing run is refused unless `-force-baseline` is explicit; a failing run
is never evidence for a baseline. A comparison
fails the command when the task stops passing, suite command failures rise,
scope/hand-edit/integrity/unexpected-change/content-check counts rise, or the
configuration fingerprint changes (the baseline is no longer comparable;
re-baseline after review). The artifact
records the exact regressions; wall-clock and changed-file counts are
deliberately not compared. A committed-file test
(`TestCommittedBaselineConfigHashes`) recomputes every entry's fingerprint from
the committed profile, task spec and suite commands, so a stale or hand-edited
`config_hash` fails `go test ./...`.

One run per task is a deterministic tripwire, not a confidence estimate. For a
noisy task, run the stripped-context control or repeat the task manually and
report the spread; never widen a budget or compare sub-command wall-clock.

## Refusal tasks

`expect_no_change: true` requires an unchanged tree. `expected_any` requires the
agent to name at least one of the listed risks (for example data loss or a
missing deprecation window), so a silent no-op does not pass.

## Collector

`agent-eval-collect.yml` runs every discovered task weekly from the trusted
main tree with `-require-baseline`, so a missing baseline entry is visible
instead of green. It evaluates each task's pinned `base_ref`, not the current
tree, so it is a drift/canary check. It is enabled with
`AGENT_EVAL_PROFILE=pinned-opencode` and
`AGENT_EVAL_MODEL_KEY` set to the model credential; the profile command maps the
latter to the provider variable opencode reads. It is scheduled and can also be
dispatched, is never triggered by a pull request, and is registered in the
nightly failure alarm and the reconcile list by `cmd/nightlyalertcheck`.

The collector is a weekly drift/canary check, not a validator of current
agent-file changes: each task pins `base_ref`, so it re-runs a frozen snapshot of
the suite, prompt and context. Keep the cadence weekly; if spend is reviewed
again, fortnightly is the next step. A change-triggered path would need a
post-merge `push` filtered to `AGENTS.md`, `.agents/skills/` and
`testdata/agent-eval/` with `-base-ref "$GITHUB_SHA"`. A `pull_request` trigger
is unsafe because the agent executes repository code with a model credential;
the supported path is PR before/after evidence plus a manual dispatch.

### Billing window

DeepSeek bills **01:00-04:00 and 06:00-10:00 UTC, Monday-Friday** at the peak
rate; all weekend (and every other hour) is off-peak at half price. The weekly
cron is Sunday 04:00 UTC, which leaves ample margin from any peak window even
when GitHub delays the start. Both `agent-eval-collect.yml` and `agent-eval.yml`
run
`.github/scripts/deepseek-offpeak.sh` before the agent is invoked: a run that
would start or extend into a peak window stops with a `::error::` and makes no
model call. A manual dispatch can pass `allow_peak: true` to accept possible
peak-rate billing deliberately; a dry-run dispatch is exempt because it costs
nothing. The guard is conservative on a weekday Chinese public holiday (which
DeepSeek bills off-peak): it has no holiday calendar, so the weekly Sunday run
is the intended path and a holiday dispatch needs `allow_peak`. The artifact's
`started_at`/`finished_at` fields are UTC, so a run's billing window can be
audited after the fact.

## Control

Dispatch experiments run a stripped-context control by default. The collector
uses `-no-control` so the weekly cost stays bounded; use the control for a
targeted investigation of whether the context caused the result.
