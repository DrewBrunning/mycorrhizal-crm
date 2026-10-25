<!--
One concern per PR. Describe what changed and why, not how.
Sign off every commit (git commit -s) — the DCO check enforces it.

The Context-change checklist is conditional: keep it only when the diff touches
`AGENTS.md`, `.agents/skills/**`, `.claude/**`, or `testdata/agent-eval/**`;
otherwise delete that whole section. Everything else is standard.
-->

## Summary



## Context-change checklist

<!--
Keep this section only when the diff touches AGENTS.md, .agents/skills/**,
.claude/**, or testdata/agent-eval/**. Delete it otherwise.
See docs/development/context-engineering.md.
-->

- [ ] `contextcheck` and the context-tooling tests pass.
- [ ] If a skill `description` or name changed, the trigger test was run and its result is below.
- [ ] Every added/changed `references/` file is linked from its `SKILL.md` and names only still-valid paths/commands.
- [ ] If a CLI/make/yarn target moved, the context references were updated (or a follow-up is filed).
- [ ] No secrets or real tokens in context files.
- [ ] Context files are ≤200 lines. 100 is a target, not a gate: no load-bearing content was cut to hit it.
- [ ] For a new nested `AGENTS.md`, `contextcheck -explain` was checked for double-load/override surprises.

Trigger-test result (prompt → skill that loaded; `not applicable` if no description/name changed):



## Implementation-workflow evidence

<!-- Standard block: see docs/development/implementation-workflow.md. -->

- [ ] Failing tests were written first; failure evidence (test names and output) is above or linked.
- [ ] Tests were reviewed before implementation: lowest layer, fails for the intended reason (not a compile error), deterministic, behaviour not implementation.
- [ ] Implementation followed TDD; no test was weakened; any test change is explained and re-verified fail-first.
- [ ] Self-review done against `docs/development/implementation-workflow.md` (correctness, completeness, coverage, safety, hygiene).
- [ ] Pre-PR gate run for the touched areas (`.github/filters.yaml`): build/lint/tests, the area coverage ratchet, and a flake-stress pass over new/changed tests.
- [ ] Coverage baseline raised in the same PR if tests were added (backend/frontend/Android).
- [ ] Generated artifacts were regenerated, not hand-edited.
- [ ] Context files updated if a cited command/path/test moved.
- [ ] One concern per PR; commits are DCO-signed; no `--no-verify`.

## Checks

Checks run (command + result):


Checks not run (exact local reproduce command for each):


<!--
The PR body is the handoff. Do not have the agent watch or poll the PR
(`gh pr checks`, sleep loops). GitHub notifies on failure: start a new session
with the failing job link and the exact reproduce command above.
-->

## Upgrade notes

<!--
Does this change affect a self-hosted operator? It does if it touches a database
migration, a configuration variable (name / default / meaning), observable
behaviour, or anything an operator must do before or after upgrading.

  - If yes: describe what the operator must do. This block is harvested verbatim
    into the release notes (REL-05, docs/changelog-policy.md). Add a
    `## Breaking changes` block too if the change is breaking.
  - If no: replace this whole section with a single line:
      no-changelog: <short reason>

Purely internal / test-only / most bug fixes need neither — just delete this
section and set the right label.
-->
