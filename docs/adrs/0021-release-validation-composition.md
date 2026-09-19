# ADR 0021: Release validation by composition, not cross-run polling

- **Status:** accepted
- **Date:** 2026-09-18
- **Depends on:** ADR 0005 (operational-event model), issue #446 (RC process), issue #447 (mandatory
  gates), issue #499 (single release workflow)
- **Supersedes:** the dispatch-and-poll orchestration introduced across #499/#543/#913/#1013/#1150
  and the post-tag metadata tail of #953/#1159
- **Implements:** issues #1161 (composable checks), #1162 (orchestrator), #1163 (Release ownership),
  #1165 (`docker-publish` gate), #1166 (docs)

## Context

The project already has a strong **gate catalogue** — per-PR checks, `push: main` checks, nightly
suites, a weekly tier, and a release-tier set — plus one human entry point, `release.yml`. That
catalogue is not the problem. The problem is **how the release path asks for those gates and waits
for their answers.**

GitHub Actions has no durable cross-run orchestration. It gives you two real dependency primitives:

- **`needs:`** — a job depends on another job *in the same run graph*;
- **`workflow_call`** — a workflow `uses:` a reusable workflow and waits for it with a real
  conclusion.

Everything else is a hand-rolled approximation. The current release path approximates cross-run
dependencies in three places by *dispatching* workflows and *polling* their check-runs or run
status:

1. `release.yml` → the mandatory per-PR gate workflows (step 3);
2. `release.yml` → the release-tier suites (step 8);
3. `release.yml` → `docker-publish.yml` → the GitHub Release (steps 24–28), where the upstream
   workflow waits for a *future* object created by a downstream workflow, then decorates it.

That approximation layer is where the defects live. Every release-path incident for months has been a
patch to it, not to a gate:

| Issue | The approximation failure it patched |
|---|---|
| #543 | A dispatched gate's check-run was structurally absent for the whole deadline |
| #913 | "Never reported" conflated with "still running" — missing vs pending |
| #1013 | Re-dispatch left duplicate check-runs; a stale `cancelled` read back as the result |
| #1150 | A `needs:`-gated fan-in job creates no check-run until its dependencies finish |
| #1142 | Interruption before the tag has no durable state — resume is heuristic |
| #1155 | The App token minted before the 75-minute poll expired before the push |
| #1158 | The 120-minute job timeout was shorter than its own sequential deadlines |
| #1160 | The documented cosign version could not verify artifacts the v3 pipeline signs |

Two structural properties make this permanent rather than incidental:

- **Observation is not a dependency.** Polling `check-runs` reconstructs a dependency the runner
  already knows internally. Each reconstruction re-introduces the same edge cases (fan-in absence,
  staleness, cancellation, disabled/renamed workflows) and each fix is another special case.
- **Forward references are the worst shape.** A workflow that must wait for a *different* workflow to
  create an object (`release.yml` waiting for `docker-publish` to create the Release) has no natural
  completion signal, so it becomes a timed segment, which then collides with token lifetimes and job
  timeouts.

### What must stay true

Any replacement must preserve:

- the **same gates** — no second definition of a check (ADR 0002's "one source of truth" instinct
  applied to CI); the release path must *compose* the existing checks, never fork them;
- the **tier model** — per-PR, on-merge-to-`main`, nightly, weekly, and on-release are different
  audiences for the same checks, and a check's tier stays declared;
- the **gate semantics** in `.github/release-gates.json` (`mandatory`, `release_gate`, tier,
  pass criterion), which `releasegatecheck` and the `main-protection` ruleset already enforce;
- the **RC mechanics** of issue #446 — an `-rc.N` tag is a pre-release, never `make_latest`, cut from
  `release/vX.Y.0`; a final release is cut from `main`;
- the **signing chain** — images and APK signed/attested; a GitHub App token is still required to
  push a tag that can trigger another workflow (`GITHUB_TOKEN` cannot).

## Decision

### 1. Every check is a reusable workflow; orchestrators compose them

Each check workflow gains `on: workflow_call` **additively** — it keeps its existing `push`,
`pull_request`, `schedule`, and `workflow_dispatch` triggers, so the per-PR ruleset contexts are
unchanged. A reusable workflow accepts a `force_all`/event-aware input so its path-gating runs the
whole suite when composed, exactly as `workflow_dispatch` already forces every area true (issue #543).

An **orchestrator** is a workflow (or a job graph) whose jobs `uses:` the member checks and gates on
`needs:`. There is exactly one definition of each check; the orchestrator chooses *which* checks a
tier runs and *waits on their real result*. Composition replaces observation.

### 2. One orchestrator per consumer, not one per check

The tiers map to orchestrators:

| Tier | Trigger | Composes |
|---|---|---|
| **per-PR** | `pull_request` (unchanged) | the checks' own `pull_request` triggers; the ruleset's required contexts |
| **on-merge-to-main** | `push: main` | the checks' own `push` triggers; an aggregator emits one durable result |
| **nightly** | `schedule` | the long suites (existing nightly tier) |
| **weekly** | `schedule` | the weekly long-poll suites |
| **on-release** | called by `release.yml` | every `release_gate: true` check **and** every release-tier suite |

The on-release orchestrator is a **reusable** workflow: `release.yml` calls it once and gates the tag
on its conclusion. `docker-publish.yml` calls the *same* reusable workflow for a tag-triggered cut
(manual re-tag / recovery), so the "is this commit releasable" definition exists once.

### 3. A release is ready when its readiness artifact says so

The on-release orchestrator emits a durable **`release-readiness` artifact** — version, source
commit, migration version, per-gate result, ASVS/adversarial acknowledgements, and the residual-risk
statement (#953). `release.yml` writes it *before* tagging; a re-dispatch reads the same artifact
instead of re-deriving state from `git` (retiring #1142's "registered but untagged = resume"
heuristic). The **tag is the "release is ready" signal**, and it is the only cross-workflow event on
the happy path.

### 4. One owner per artifact

The workflow that creates an object owns everything attached to it:

- `docker-publish.yml` **owns the Release** — assets, `SHA256SUMS`, provenance, the cosign bundle,
  and `release-metadata.json`. `release.yml` ends at the tag and never waits for a Release.
- `release.yml` **owns the candidate decision** — validate, compose the gates, regenerate the schema
  fixture, write the readiness artifact, tag.

Where a staged flow is unavoidable, each stage triggers the *next* on a completion event carrying the
readiness artifact — never an upstream poll of a not-yet-existing downstream object.

### 5. Credentials are minted at the point of use, per owner

Every App-token mint moves to immediately before its single write, and each workflow holds only the
permissions of its own jobs. The release composer is read-only at the **top level**, with write
scopes declared only on the job that needs them (Scorecard's highest-scoring Token-Permissions
shape). GitHub validates nested-job permissions statically across the whole reusable-workflow chain,
so a scope a member check declares must be granted at every calling job above it: the three
SARIF-uploading scans (`sast`, `container-hardening`, `zizmor`) keep their job-level
`security-events: write`, and the call-jobs in `release-validate.yml` plus the two jobs that call it
(`release.yml`'s `validate`, `docker-publish.yml`'s `release-gate`) grant it so the chain validates.
The upload *step* in each scan is still gated on `github.event_name != 'workflow_call'`, so the
scope is inert in a composed run; the upload only happens in each scan's native push/PR/schedule
run. The publish workflow's `packages`/`id-token`/`attestations` writes stay scoped to publish and
are never granted to the release job. This is recorded in the `asvs-l2-verification-report.md` §9
privileged-CI-credential row in the same change.

### 6. One release at a time

A `concurrency` group keyed by the release version serializes cuts; a superseded run is cancelled, so
the readiness artifact and the tag can never race.

## Consequences

- **Deleted:** the dispatch-and-poll code in `release.yml`/`docker-publish.yml`, and the release-gate
  state/decide shell scripts plus their tests. The special
  cases #913/#1013/#1150 stop existing as code because they stop existing as a problem.
- **Deleted:** the three 40-minute metadata-attach segments, the extra App-token re-mints, and the
  360-minute job timeout (#1159's stopgap can be lowered once the tail is gone).
- **Changed:** `.github/release-gates.json`'s `tier`/`release_gate` fields become the composition
  manifest; `governancecheck`/`releasegatecheck` assert that the on-release orchestrator's `needs:`
  graph covers every `release_gate: true` gate and includes the declared release-tier set. The gate
  table in `docs/development/release-gates.md` documents composition instead of polling.
- **Unchanged:** the per-PR ruleset and its required contexts; the RC tag semantics (#446); the
  signing chain; `min-version-tests`/`zap-dast` remaining release-tier.
- **Cost / limits:**
  - Reusable workflows nest at most **4 levels**, and a called workflow's `permissions` cannot exceed
    the caller's — hence Decision 5's top-level-read-only composer, with the SARIF `security-events:
    write` grant propagated to every calling job and the upload step itself gated off when composed.
  - A composed check runs *inside the caller's run*, so the release run's job count grows; that is
    the point (it is now one observable graph) and the 6-hour job ceiling is no longer load-bearing.
  - The GitHub App token is still required to push a tag that triggers `docker-publish.yml`.
  - `workflow_call` is additive, so per-PR timing and path-gating are unchanged; a check that only
    ever runs composed is a check whose cheap PR signal was lost — none here are.

## Alternatives considered

- **`workflow_run`-chained stages.** Rejected. It replaces one observable state machine with several
  unobservable ones, fires on the *default branch's* copy of the workflow file, cannot be a required
  status context with the same fidelity, and has chaining-depth/secret semantics that make the same
  class of bug harder to reason about, not easier. It is more "spin off a job and hope", which is the
  protein of the current problem.
- **One monolithic workflow for everything.** Rejected. It would duplicate the per-PR checks (drift —
  the explicit thing to avoid), lose their path-gating and ruleset identities, and concentrate every
  privileged token in one place.
- **An external orchestrator** (a Go tool driving `gh` and `gh api` state, or an issue/PR as the
  state machine). Rejected for now: new infrastructure and another privileged credential to secure,
  for a benefit `workflow_call` already captures inside GitHub. Revisit only if a future gate cannot
  be expressed as a workflow.
- **Keep dispatch-and-poll and keep patching it.** Rejected by the incident table above.

## Migration plan

Phased, each independently shippable and revertible (issues #1161–#1166):

1. Make the check workflows callable (`workflow_call` + force-all) — no behavior change.
2. Add the on-release orchestrator and the per-tier aggregators; wire `release.yml` to compose the
   mandatory and release-tier checks; delete the two poll scripts.
3. Move Release ownership (metadata attach, `SHA256SUMS`, smoke) into `docker-publish.yml`; end
   `release.yml` at the tag.
4. Add the durable `release-readiness` artifact, the concurrency group, and retire the resume
   heuristic.
5. Reconcile `docker-publish.yml`'s `release-gate` onto the same reusable orchestrator.
6. Update `.github/release-gates.json`, `docs/development/release-gates.md`, and the ASVS §9 row to
   describe and enforce composition.
