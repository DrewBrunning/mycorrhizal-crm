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
cd backend && go run ./cmd/agenteval -task database-migration -profile pinned
```

Explicit `-agent-command`/`-agent-model`/`-agent-version` flags cannot be
combined with `-profile`; a free-form run is not comparable with a baseline.
`dry-run` profiles are for smoke tests only and are rejected when a baseline is
involved.

## Baseline

`baseline.json` records the last accepted run per profile and task: success,
suite commands passed/failed, scope violations, generated-artifact hand-edits
and a fingerprint of the agent, prompt, base ref and suite. A missing entry is
"no baseline", never a regression by itself.

Compare a run against it:

```bash
cd backend && go run ./cmd/agenteval -task database-migration -profile pinned \
  -baseline testdata/agent-eval/baseline.json -no-control
```

Pass `-require-baseline` to fail when the profile/task has no entry (the
collector does). Promote a known-good run and commit the diff:

```bash
cd backend && go run ./cmd/agenteval -task database-migration -profile pinned \
  -write-baseline testdata/agent-eval/baseline.json -no-control
```

A non-passing run is refused unless `-force-baseline` is explicit. A comparison
fails the command when the task stops passing, suite command failures rise,
scope/hand-edit counts rise, or the configuration fingerprint changes (the
baseline is no longer comparable; re-baseline after review). The artifact
records the exact regressions; wall-clock and changed-file counts are
deliberately not compared.

## Refusal tasks

`expect_no_change: true` requires an unchanged tree. `expected_any` requires the
agent to name at least one of the listed risks (for example data loss or a
missing deprecation window), so a silent no-op does not pass.

## Collector

`agent-eval-collect.yml` runs every discovered task weekly against the trusted
main tree with `-require-baseline`, so enabling it without a promoted baseline
is visible instead of green. It is disabled until the `AGENT_EVAL_PROFILE`
repository variable names a non-dry-run profile in `agents.yaml`; set
`AGENT_EVAL_MODEL_KEY` for the agent subprocess. It is registered in the nightly
failure alarm and the reconcile list by `cmd/nightlyalertcheck`.

## Control

Dispatch experiments run a stripped-context control by default. The collector
uses `-no-control` so the weekly cost stays bounded; use the control for a
targeted investigation of whether the context caused the result.
