# Agent evaluation tasks

`cmd/agenteval` runs one representative task per registered suite in a
disposable worktree and scores the result. It is dispatch-only (or weekly via
`agent-eval-collect.yml`) and never a merge or release gate. See
`docs/development/context-engineering.md`.

## Pinned profiles

`agents.yaml` holds the pinned agent configurations. A profile names the
command, model and CLI version, and a real profile's command must contain
`{prompt}`. Use it instead of free-form overrides:

```bash
cd backend && go run ./cmd/agenteval -task database-migration -profile pinned
```

`-agent-command`/`-agent-model`/`-agent-version` still override a profile for a
one-off experiment, but that run is not comparable with a baseline.

## Baseline

`baseline.json` records the last accepted run per profile and task: success,
suite commands passed, scope violations and generated-artifact hand-edits. A
missing entry is "no baseline", never a regression.

Compare a run against it:

```bash
cd backend && go run ./cmd/agenteval -task database-migration -profile pinned \
  -baseline testdata/agent-eval/baseline.json -no-control
```

Promote a known-good run by writing it and committing the diff:

```bash
cd backend && go run ./cmd/agenteval -task database-migration -profile pinned \
  -write-baseline testdata/agent-eval/baseline.json -no-control
```

A comparison fails the command when the task stops passing, fewer suite
commands pass, or scope/hand-edit counts rise. The artifact records the exact
regressions; wall-clock and changed-file counts are deliberately not compared.

## Collector

`agent-eval-collect.yml` runs every discovered task weekly against the trusted
main tree. It is disabled until the `AGENT_EVAL_PROFILE` repository variable
names a profile in `agents.yaml`; set `AGENT_EVAL_MODEL_KEY` for the agent
subprocess. It is registered in the nightly failure alarm and the reconcile
list by `cmd/nightlyalertcheck`.

## Control

Dispatch experiments run a stripped-context control by default. The collector
uses `-no-control` so the weekly cost stays bounded; use the control for a
targeted investigation of whether the context caused the result.
