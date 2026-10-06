---
title: Release gates
parent: Development
nav_order: 8
---

# Release gates

**This is the canonical mandatory-gate statement (REL-03, issue #447).** It defines which checks
must pass before a `v*` tag is allowed to publish, at which **tier**, with what explicit
**pass/fail criterion**, and how a failure actually blocks publication rather than being noticed
afterward.

The machine-readable twin is [`.github/release-gates.json`](https://github.com/DrewBrunning/mycorrhizal-crm/blob/main/.github/release-gates.json).
`cd backend && go run ./cmd/releasegatecheck` (a `Docs & security-doc citations` step, every PR)
fails the build if the JSON is malformed, if any gate names a workflow that does not exist, or if
this table and the JSON disagree. It also asserts every workflow a `release_gate:true` or
release-tier gate names is **composable** (declares `workflow_call`) **and that the composer
`release-validate.yml` calls exactly that set** — no omission, no extra — so the release
orchestrator of [ADR 0021](../adrs/0021-release-validation-composition.md) cannot drift from the
registry and needs no dispatch-and-poll. The `release-gate` job in `docker-publish.yml` reads the
same JSON.

This page is contributor- and maintainer-facing; it assumes a repo checkout and the Go toolchain
for the `go run` command above.

## Tiers

| Tier | Meaning |
|---|---|
| **per-pr** | Fast enough for every pull request. Blocks merge via the `main-protection` ruleset, and is **re-checked on the release commit** by the `release-gate` job (for the subset marked `release_gate` in the JSON). |
| **release-internal** | A job inside `docker-publish.yml`. Enforced by that workflow's `needs:` graph — if it fails, no release, no images, no APK. |
| **release-tier** | Too slow for every PR (nightly / `push: main` / on-dispatch). The [REL-06 release workflow (#499)](https://github.com/DrewBrunning/mycorrhizal-crm/issues/499), `release.yml`, triggers the ones with no `push: main` trigger (`min-version-tests`, `zap-dast`) and waits on every release-tier run for the release commit before it pushes the tag — an observed failure means the tag is never pushed, a 75-minute deadline with a run still going is a `::warning::` and the tag proceeds. Because a failure at this stage leaves the fixture commit on `main` with no tag, `release.yml` is re-entrant there: re-dispatching the same version resumes at the new `main` tip ([#1142](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1142)). |
| **advisory** | Runs and is visible, but a failure does not block a release. A regression is triaged, not gating. |

## How publication is blocked

Four mechanisms, in order of when they fire:

1. **Merge time** — the `main-protection` branch ruleset requires the per-PR check contexts, so a
   failing gate cannot reach `main` in the first place (the merge-time counterpart is
   [#508](https://github.com/DrewBrunning/mycorrhizal-crm/issues/508)).

2. **Cut time** — [`release.yml`](https://github.com/DrewBrunning/mycorrhizal-crm/blob/main/.github/workflows/release.yml)
   (REL-06, #499) is the single human action that cuts a release. Its `preflight` job runs
   `go run ./cmd/citecheck` and `go run ./cmd/releasegatecheck` directly; its `validate` job then
   **composes every gate** through the reusable
   [`release-validate.yml`](https://github.com/DrewBrunning/mycorrhizal-crm/blob/main/.github/workflows/release-validate.yml)
   — the `release_gate: true` per-PR checks and the release-tier suites are invoked with `needs:`
   dependencies as the *same workflow files* the per-PR/`push: main`/schedule paths run, so there is
   one definition of each check and nothing to poll (ADR 0021, issue #1162). A failure stops the run
   before the tag. Two release-only obligations are enforced in the `release` job before the tag: the
   ASVS/MASVS re-verification changelog row ([#608](https://github.com/DrewBrunning/mycorrhizal-crm/issues/608))
   and the per-release adversarial delta ([#953](https://github.com/DrewBrunning/mycorrhizal-crm/issues/953)),
   each with a recorded `ack_*` dispatch escape. `dry_run: true` runs the whole battery and the
   fixture regeneration but makes no commit, push, or tag, and is exercised weekly by
   `release-dry-run.yml` ([#929](https://github.com/DrewBrunning/mycorrhizal-crm/issues/929)).

3. **Publication time** — `docker-publish.yml`'s **`release-gate`** job calls the *same*
   `release-validate.yml` before anything publishes (ADR 0021, issue #1165). Because the tag-triggered
   path and the cut-time path compose one workflow, they cannot drift. `create-release`,
   `build-android-apk`, and `schema-fixture-gate` `needs:` the gate (or the sibling
   `release-gate-override` job), so a failed gate blocks publication. The **only** way past it is to
   run `docker-publish.yml` from the Actions tab (**Run workflow**) with a non-empty
   **`override_reason`**; the gate is skipped, `release-gate-override` records
   `RELEASE GATE OVERRIDDEN by <actor>: <reason>` to the run summary, and publication proceeds —
   explicit, logged, attributed. A tag **push** has no override path.

4. **The `needs:` graph** — the `release-internal` gates enforce themselves: `build-and-push`
   `needs: schema-fixture-gate` + `build-android-apk`, `create-release` `needs: build-android-apk`,
   `verify-release-assets` is the final belt-and-suspenders check that the APK, its cosign bundle,
   the SLSA provenance, `release-metadata.json` and `SHA256SUMS` actually landed on the Release and
   every image tag resolves.

**No gate is polled.** ADR 0021 replaced the dispatch-and-poll layer (and its
`release-gate-state.sh` / `release-gate-decide.sh` special cases for missing check-runs, stale
re-dispatches, and `needs:`-gated fan-in jobs) with `workflow_call` composition. See
[ADR 0021](../adrs/0021-release-validation-composition.md) for why.

### The manual-dispatch fallback (issue #1396)

`docker-publish.yml` can also be run by hand (`workflow_dispatch` with the `tag` input) for a tag whose
push event never produced a run. That path runs `main`'s copy of the workflow against the tag's code. It
**cannot** produce everything a tag push does:

| Job | On the fallback | Why |
|---|---|---|
| `apk-provenance` | **skipped** | the SLSA generator records the triggering ref; a dispatch from a branch is a branch ref that `slsa-verifier --source-tag` rejects, so no verifiable provenance can be made |
| `create-release` | skipped | the Release already exists; re-uploads use `--clobber` |
| `verify-release-assets` | **runs** | rebuilds `SHA256SUMS`, checks every asset (incl. the attached APK's `libmycorrhizal.so`) and the images. With no `mycorrhizal-apk.intoto.jsonl` it **fails** and says the release is not promotable |
| `post-publish-smoke` | runs | it only needs the published image |

So a release published **only** via the fallback concludes **red**, not green, and `promote-rc.yml` refuses
to promote an RC whose Release lacks the provenance or `SHA256SUMS`. Recover by cutting the next release
candidate, or — once the workflow at the tag's commit is fixed — by deleting and re-pushing the tag so a
push-triggered run produces the provenance.

`go run ./cmd/releasegatecheck` enforces the rule: a **mandatory** release-internal gate in
`docker-publish.yml` may not carry a `github.event_name == 'push'` guard unless it is listed in
`releasegates.PushOnlyAllowlist` with a reason **and** a compensating job that runs on dispatch and
asserts its absence with an `::error::` (`backend/internal/releasegates/dispatchpath.go`). Today only
`apk-provenance` is allowlisted, compensated by `verify-release-assets`.

### Build once: the tested image is the shipped image (issue #1484)

Before #1484 the artifact the gates ran was not the artifact that shipped: `e2e-tests.yml` built
`mycorrhizal-crm-test:latest` from source, and `docker-publish.yml` built the release image again
later, so a green battery said nothing about the bytes published. Now:

1. **`build-candidate`** (a job inside `release-validate.yml`) builds the all-in-one image **once** —
   same Dockerfile, build args, OCI labels, `SOURCE_DATE_EPOCH` and multi-arch platforms as the release
   build; the stamp arithmetic is the shared `.github/scripts/release-image-stamp.sh` so the two cannot
   disagree — and pushes it to GHCR as `ghcr.io/<repo>:candidate-<sha7>` (a non-release tag; no `latest`,
   no version tag). Its **digest** is the workflow's `candidate_digest` output.
2. Every gate that runs the image — `e2e-tests.yml` (all three jobs), `container-hardening.yml`,
   `zap-dast.yml`, and the new release-tier `deploy-smoke.yml` — takes an optional `image_digest`
   input. Set (only the release composition sets it), the gate **pulls that digest** with
   `.github/scripts/candidate-image.sh pull` (which refuses anything but a `sha256:` digest and
   verifies the pulled image carries it) and skips its from-source build; every gate's log shows the
   same digest. Per-PR, `push: main`, nightly and dispatch runs pass nothing and keep building from
   source. `deploy-smoke.yml` boots it through `docker-compose.yml` + `docker-compose.candidate.yml`
   (`build: !reset null`, so Compose cannot silently rebuild). `chaos-tests.yml` does not run the
   container image (Go tests only) and is unaffected.
3. **`docker-publish.yml`'s `build-and-push`** receives the digest from its `release-gate` job and,
   for the all-in-one image, **re-tags it by digest** (`docker buildx imagetools create`, the
   `promote-rc.yml` pattern) instead of rebuilding; each resulting tag is resolved back and asserted
   equal to the tested digest, then cosign/attestation/SBOM run against that digest as before. The
   backend and frontend images (which no gate runs) still build in that job.
4. **`verify-release-assets`** asserts `published digest == tested digest` and fails with an
   `::error::` otherwise (or if the gate passed but exported no digest).
5. `post-publish-smoke` stays as defense in depth, but it is no longer the first time the published
   image is exercised.

Two honest limits. **(a)** `release.yml` validates the release commit *before* the fixture commit and
tag exist, so its pre-tag battery tests a candidate built from that commit; `docker-publish.yml`'s
`release-gate` then builds and tests a candidate from the **tagged** commit, and *that* digest is what
publishes. `release.yml` records its own candidate in `release-readiness.json` (`validated_candidate`)
for comparison; the two coincide whenever the fixture commit does not change the image build context.
When the tag-time battery is *reused* (issue #1487, below), the carried gates cover the pre-tag candidate's
tree and only `deploy-smoke` boots the digest that publishes.
**(b)** With the gate overridden (`override_reason`) there is no tested candidate, so the all-in-one
image builds from source and `verify-release-assets` only warns that digest equality cannot be asserted.

`go run ./cmd/releasegatecheck` enforces the wiring (`releaseworkflow.CheckCandidate`): `build-candidate`
exists and exports the digest, every image-running gate declares `image_digest`, is called with
the digest and `needs: build-candidate`, and pulls through the script; the candidate and
`build-and-push` agree on build args and the shared stamp; and the retag and digest-equality
assertions are present. A candidate tag is left in GHCR per release attempt (including dry runs);
prune old `candidate-*` package versions periodically.

## Flake exposure: ledger, reuse, rerun, retries (issue #1487)

Release reliability data (the last 50 `release.yml` runs as of 2026-10-05: 19 success, 18 failure,
8 cancelled, 5 `startup_failure`) showed the failures after `preflight` were almost all flakes in
individually-gating suites, not defects in the candidate — and the battery gave every flaky suite
**two** rolls per release (once in `release.yml`'s `validate`, again in `docker-publish.yml`'s
`release-gate` after the tag exists, where a failure leaves a tag with no artifacts) and re-ran the
**whole** battery on any failure. Four mechanisms cut that exposure without weakening what a gate
means. The decision logic is unit-tested Go (`backend/internal/releaseplan`, front end
`cd backend && go run ./cmd/releaseplan`), not inline shell.

### The per-gate ledger

`release-validate.yml`'s `results` job writes `release-gate-ledger.json` into the
`release-gate-results` artifact: one row per composed gate (every job that calls a reusable
workflow; `build-candidate` is not a gate).

```json
{"sha": "<commit the battery ran against>", "release_tag": "v1.2.3", "gate": "e2e-tests",
 "conclusion": "success", "run_id": "123", "carried_from_run": "98"}
```

`carried_from_run` appears only on a gate this run did **not** execute but carried from an earlier
run; `run_id` is then the current run and `carried_from_run` the run that earned the success. A
carried row keeps the *earning* commit's `sha`, so the ledger can never claim a commit it did not
test. A gate that did not run for any other reason records `skipped`, which no later carry or reuse
ever accepts. `release.yml` folds the ledger into `release-readiness.json` as `gate_ledger`.

### Tag-time reuse (`docker-publish.yml`'s `reuse-decision`)

The tag push used to re-run the entire battery that `release.yml` had just passed. Now a
`reuse-decision` job reads the ledger from the `release-readiness-<tag>` artifact and runs
`releaseplan reuse`. The pre-tag battery is **accepted** only when **all** hold:

1. every composed gate is a recorded `success` for this tag, and the ledger covers exactly **one**
   validated commit;
2. that commit is an ancestor of the tag (`git merge-base --is-ancestor`); and
3. the tree diff from the validated commit to the tag (`git diff --name-only`) is a subset of the
   fixture allowlist — exactly the two files `release.yml`'s fixture commit may change:
   `backend/database/testdata/schemas/<tag>.sql` and `backend/internal/schemafixture/releases.go`
   (an RC tag *is* the validated commit: empty diff).

Anything else — no artifact (a hand-pushed tag, expired, pre-ledger), a gate not green, a stray file,
an unreadable diff — is `reuse=false` and the **full battery runs, exactly as before**: the tool never
errors toward reuse, and the job is `continue-on-error` so it can only ever cost the reuse, never block
a release. On reuse, `release-gate` is called with `skip_gates` (every gate except the retest floor)
and the carried rows; the composer's per-gate `if:` skips them. The decision — reuse or not, the
reason, the changed files, which gates were carried — is written to the run summary and recorded in
the attached `release-readiness.json` as `tag_time_battery`.

**The retest floor is `deploy-smoke`.** `build-candidate` still builds the image from the **tag** (its
digest is what `build-and-push` re-tags and `verify-release-assets` compares), and that fresh build
differs from the pre-tag candidate by its stamped commit and the inert fixture files, so under reuse
the exact published digest was *not* exercised by the carried gates. The clean-install smoke is cheap
and boots exactly those bytes, so it always re-runs (`releaseplan.RetestOnReuse`). This is the
honest weakening of #1484's "tested image == shipped image" under reuse: the carried gates vouch for
the **tree** (identical but for the allowlisted fixture), the smoke for the **bytes**. A partial
policy ("run only the gates whose inputs differ" when the diff is *not* fixture-only) was deliberately
not built: a hand-kept path-to-gate map is exactly the kind of table that silently goes stale, and a
source change in the diff simply runs everything.

### `rerun_gates` on `release.yml`

`release.yml` accepts `rerun_gates` — a comma list of gate ids (the job ids in
`release-validate.yml`) or the word `failed` — and optional `rerun_from_run`. `preflight` finds the
newest `release-gate-results` ledger recorded for the release commit (or the named run's), runs
`releaseplan rerun`, and `validate` is called with `skip_gates` and the carried rows. Rules, all
pinned by `releaseplan`'s tests: only a recorded `success` for **exactly this commit and this version**
is carried; a gate with no such row re-runs even if not named; an unknown id, or a request with
nothing left to run, fails in `preflight` before any gate; a ledger for a different commit carries
nothing. `build-candidate` always rebuilds (cache-warm) so re-run image gates test the digest the
readiness record names. Ledgers live in a 5-day artifact; past that, dispatch without `rerun_gates`.
Dependent gates are handled explicitly: `reference-clients-e2e` needs `android-tests`, so its `if:`
accepts a *skipped* (carried) `android-tests` while a *failed* one still blocks it.

### Flake policy: what is retried, what is not

Evidence first (failed gate step on each of the failing runs, from the Actions API):

| Suite | Failure | Class | Policy |
|---|---|---|---|
| ZAP `zapgate` (2 runs) | `self-test: no High/Medium alert for plugin 40012 found on the canary — the scan is blind` | **Scanner flake** (ZAP's JVM drops in-progress alert data under memory pressure, issue #1278); the app was not judged | `zapgate` exits `3` for *exactly* this case; `.github/scripts/zap-scan-gate.sh` re-scans **once**. An unaccepted High/Medium **app** finding exits `1` and is **never** retried. |
| Schemathesis `schemagate` (2 runs) | `1 unaccepted finding(s)`, a randomized-fuzz `Server error` (5xx) | **A real finding**, not infra: `schemagate` already gates on a committed ignore list and failed on a *new* finding | **Not retried.** Re-rolling a fuzzer until it stops finding a 500 hides a defect; fix the 5xx or add a justified ignore entry. |
| `min-version-tests` Go floor (3 runs, 2026-09-19..22) | a different timing-sensitive test each time: `TestCheckDBIntegrityScheduledFiresWebhookOnCorruption` (SQLITE_BUSY lock-release race, with `services` taking 2–3 min under a contended runner), `TestDetectReachOutSuggestions_OrganizationChange`, and a `schemafixture` 10 min timeout (since raised to 25 min) | **Timing flakes, not floor-specific** — the same packages pass at the current toolchain in the same battery | `.github/scripts/go-test-retry-failed.sh` re-runs **only the failed packages, once**, with a `::warning::` naming each retried package and test. A build/vet failure, more than 3 failed packages, or a package that fails twice still fails the leg. |
| Playwright E2E, DAVx5 / Android emulator legs, zizmor, large-dataset migration | already retried in-job (the emulator legs run a "retry, fresh emulator" attempt) or one-off | — | unchanged; the ledger + `rerun_gates` bound their cost to one gate instead of the battery |

The two retries are *bounded* on purpose: a retry that can mask a real finding turns a gate into a
suggestion. The structural wiring (the scripts are what the workflows run; `zapgate`'s exit code and
the script's agree) is pinned by `releaseworkflow.CheckResilience` and the shell tests
`.github/scripts/tests/zap-scan-gate.test.sh` / `go-test-retry-failed.test.sh` (CI: `actionlint.yml`).

`go run ./cmd/releasegatecheck` enforces the wiring (`releaseworkflow.CheckResilience`): every
composed gate carries a `skip_gates` condition naming itself, dependent gates tolerate a skipped
need, `build-candidate` is never conditional, the `results` job records and uploads the ledger,
`release.yml` plans the rerun and passes it to `validate`, `docker-publish.yml`'s `reuse-decision`
exists, is `continue-on-error`, checks ancestry and feeds `release-gate`, and the two retry wrappers
are what `zap-dast.yml` and `min-version-tests.yml` run.

**Not done here.** PR and push CI still retry once and nightly retries zero, so a flaky test is masked
(PR) or loud-then-closed (nightly) with no longitudinal signal; a flake-rate record per suite is a
separate piece of work. The "release success rate over the next 10 dispatches" in the issue's verify
criteria can only be measured after this lands and is reported on the gate issue.

## The Android decision (issue #527)

**A release hard-blocks on a green, keystore-signed, `apksigner`-verified APK that lands on the
GitHub Release.** `create-release` and `build-and-push` both `needs: build-android-apk`, so a
signing or build failure blocks the entire release — Docker images included. This deliberately
reverses the earlier "a signing problem here shouldn't block the Docker images" decoupling:
`v0.7.0`'s premise is that a release is one artifact set, and the Android client is part of it.

`build-android-apk` now, after `:app:assembleObtainiumRelease`:

- runs **`apksigner verify --print-certs`** and fails if the APK is not signed (v2/v3 scheme).
  When the repo variable **`ANDROID_SIGNING_CERT_SHA256`** is set, it also asserts the signer
  certificate SHA-256 matches; otherwise it warns that the cert is not pinned. Set that variable
  to the release keystore's cert fingerprint to enforce.
- asserts the built APK's **`versionCode`** equals the value the workflow computed
  (`1000 + GITHUB_RUN_NUMBER`, strictly increasing per [the versioning policy](../versioning-policy.md))
  and is `> 1` — a light pin on the plugin's override wiring so an in-place upgrade keeps
  working. It does not install anything.
- asserts `app-obtainium-release.apk` and `mycorrhizal-apk.sigstore.json` are present on the Release.

**Known gap, accepted (issue #994):** no gate here, or anywhere in CI, drives the
release-signed/R8-minified APK through an instrumented test, and none installs release N then
N+1 over it to confirm the offline mirror survives — the "full emulator test" #480 originally
named. [#480](https://github.com/DrewBrunning/mycorrhizal-crm/issues/480) (ANDROID-03) landed
`RoomMigrationEncryptedTest`, which proves the Room migration chain against a real
SQLCipher-encrypted file, and `MigrationVersionCoverageTest`, which guards that every version
pair has a registered migration or a recorded destructive-fallback decision — but both run on
the **debug** variant via the `android-e2e` job, not the minified release artifact, and neither
performs a real two-APK install. [PR #804](https://github.com/DrewBrunning/mycorrhizal-crm/pull/804)
deliberately did not build the literal install-N-then-install-N+1 harness: `versionCode` monotonicity
(asserted above) is what makes Android accept the install, which is orthogonal to whether the
database migrates correctly once the new code runs — the thing the JVM/instrumented migration
suite already proves directly. Disposition: accept, not built — see the "E2E Android
(instrumented)" section of [`testing.md`](testing.md#e2e-android-instrumented) for where this is
tracked.

**Known gap, accepted, with a manual gate (issue #1339): Android local-only mode.** The embedded
server ships arm64-only, so `LocalOnlyModeE2eTest` (the only end-to-end proof of `LocalServerHost`:
process exec, Keystore-wrapped secrets, readiness handshake, `/health` over the socket) is an
`assumeTrue` **skip** on the x86_64 `Android E2E (emulator)` gate. A skip is not evidence, so that
green check says nothing about local mode. Two things cover it instead:

- **Automated, per-PR:** `backend/main_test.go` builds the real backend binary for the CI host,
  execs it with `--embedded-host`, and asserts the readiness handshake, `/health` over the Unix
  socket, an authenticated call with the minted token, and clean SIGTERM shutdown; it also asserts
  the no-arg exec fails, and pins the Kotlin `EMBEDDED_HOST_ARG` to the Go `embeddedHostArg`. This
  is the `Backend (Go)` job, so it is merge-blocking. It proves the binary's contract, not the
  Android host or seccomp behaviour.
- **Automated, per release build (issue #1390):** the embedded server is only packaged when
  Gradle gets `-PMYCORRHIZAL_BUILD_EMBEDDED_SERVER=true` with Go on the builder, and a build
  without it silently hides local mode. So `docker-publish.yml` (signed release APK),
  `android-apk-build.yml` and `android-aab-build.yml` each run `actions/setup-go`, pass the flag
  (a missing Go then fails the Gradle build instead of skipping), and hard-assert that the
  finished artifact contains `lib/arm64-v8a/libmycorrhizal.so` (`base/lib/arm64-v8a/…` in the
  AAB); `verify-release-assets` re-checks the APK attached to the GitHub Release. The F-Droid
  FOSS build does not carry it yet (see [`fdroid.md`](fdroid.md)).
- **Manual, per release candidate:** before dispatching `release.yml`, run
  `./gradlew :app:connectedObtainiumDebugAndroidTest -Pandroid.testInstrumentationRunnerArguments.class=com.mycorrhizal.crm.e2e.LocalOnlyModeE2eTest` on a real
  **arm64 device** (the Pixel 8a runbook in [`README-developer.md`](../../README-developer.md)) with
  `-PMYCORRHIZAL_BUILD_EMBEDDED_SERVER=true` (otherwise no `libmycorrhizal.so` is packaged and the
  test skips) and confirm the test *ran* (not "skipped"). Do not install over the production package
  `com.mycorrhizal.crm`; use a suffixed debug appId (e.g. a temporary `applicationIdSuffix`), because
  `connectedAndroidTest` uninstalls the app it installed.

  Separately confirm the **release-built** APK contains the binary:
  `unzip -Z1 app-obtainium-release.apk | grep -x lib/arm64-v8a/libmycorrhizal.so` (the release
  workflow asserts this, so on a tagged build this is a re-check of the attached asset).

**PKCS12 keystore note:** `SIGNING_KEY_PASSWORD` **must equal** `SIGNING_STORE_PASSWORD` for this
keystore. A mismatch fails `:app:assembleObtainiumRelease` with an opaque padding error, not a clear message.

## The gates

Generated view of `.github/release-gates.json`. `releasegatecheck` asserts every row here has a
matching registry entry (name, tier, mandatory) and every `workflow` file exists.

<!-- release-gates:begin -->

| Gate | Tier | Mandatory | Pass criterion | Workflow |
|---|---|---|---|---|
| `Backend (Go)` | per-pr | yes | go build + go vet + gofmt clean and `go test ./... -race` passes; no package exceeds its -timeout. | `unit-tests.yml` |
| `Frontend (Vitest)` | per-pr | yes | `tsc --noEmit` and `vitest run` both pass. | `unit-tests.yml` |
| `Frontend bundle-size budget` | per-pr | yes | `yarn build && yarn budget` passes: no emitted chunk grows past frontend/bundle-budget.json's tolerance (#556). Merge-time only; path-gated like the other unit-tests.yml required contexts. | `unit-tests.yml` |
| `Run E2E Tests` | per-pr | yes | the route-stubbed Playwright suite (including @perf specs) passes. | `e2e-tests.yml` |
| `Android (Gradle)` | per-pr | yes | `testDebugUnitTest` (libraries) plus the flavor-qualified `:app:testObtainiumDebugUnitTest`/`:app:testFossDebugUnitTest`, `lintDebug` plus `:app:lint{Obtainium,Foss}Debug`, `detekt`, and the flavor-qualified debug assembles all pass. | `android-tests.yml` |
| `Android E2E (emulator)` | per-pr | yes | the instrumented suite passes against the docker-compose.test.yml backend on an API-35 emulator. | `android-tests.yml` |
| `Android scan (mobsfscan)` | per-pr | yes | mobsfscan reports no new high-severity finding on the Android sources. | `sast.yml` |
| `Scan workflows (zizmor)` | per-pr | yes | zizmor exits 0 — no finding above what zizmor.yml's ignore list accepts. | `zizmor.yml` |
| `CIS container hardening scan` | per-pr | yes | the all-in-one image passes docker/cis-hardening.sh and the Trivy misconfig/secret scan with no CRITICAL/HIGH. | `container-hardening.yml` |
| `codecov/patch/backend` | per-pr | yes | changed Go lines are >= 95% covered (codecov.yml). Merge-time only: a PR-diff concept, not re-polled at publication. | `unit-tests.yml` |
| `codecov/patch/frontend` | per-pr | yes | changed TypeScript lines are >= 90% covered. Merge-time only. | `unit-tests.yml` |
| `codecov/patch/android` | per-pr | yes | changed Kotlin lines are >= 80% covered. Merge-time only. | `android-tests.yml` |
| `Detect Changes` | per-pr | yes | the shared path-filter job completes; always green (structural). Required so path-skipped suites can be required checks (#264). | `unit-tests.yml` |
| `Docs & security-doc citations` | per-pr | yes | citecheck + depexceptions + deprecations + docscheck + releasegatecheck all exit 0. Runs on every PR and nightly. `release_gate: true` — composed by the release gate and re-asserted at cut time, where `release.yml` also runs `citecheck` directly as a hard gate (#608). | `unit-tests.yml` |
| `Migration Tests` | per-pr | yes | every supported-release upgrade leg, adjacent hop, and down round-trip passes. Per-leg check names make polling impractical; the release commit only adds a frozen schema dump, which schema-fixture-gate verifies, and the push:main run covers the chain. | `migration-tests.yml` |
| `Go binary reproducible` | per-pr | yes | two builds from different paths are byte-identical (REL-04). Runs on the release commit's push:main; not in the ruleset. | `reproducibility.yml` |
| `validate-tag` | release-internal | yes | the pushed tag matches the versioning-policy pattern (REL-01, backend/internal/versionpolicy). Blocks every downstream job. | `docker-publish.yml` |
| `release-gate` | release-internal | yes | calls the reusable `release-validate.yml` and blocks publication unless it passes: every `release_gate:true` per-PR check and every release-tier suite is composed with `needs:` (ADR 0021, issue #1165), so the tag-triggered path cannot drift from `release.yml`'s cut-time composition. A `workflow_dispatch` run with a non-empty `override_reason` skips the composed gate and the sibling `release-gate-override` job records the override with the actor. | `docker-publish.yml` |
| `schema-fixture-gate` | release-internal | yes | a committed backend/database/testdata/schemas/<tag>.sql exists for a mycorrhizal-supported-series tag (MIG-01, #436/#529). | `docker-publish.yml` |
| `build-candidate` | release-internal | yes | a job inside the composer `release-validate.yml` ([#1484](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1484)): builds the all-in-one image once (multi-arch, the release's own build args, labels and `SOURCE_DATE_EPOCH`), pushes it as a non-release `candidate-<sha>` tag and exports its digest. Every image-running gate pulls that digest instead of rebuilding; a failed build blocks the battery. | `release-validate.yml` |
| `build-and-push` | release-internal | yes | the multi-arch images publish; each digest gets a cosign keyless signature, an SBOM, and SLSA build provenance. The all-in-one image is **not rebuilt**: its release tags are created by re-tagging the candidate digest the release-gate tested ([#1484](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1484)); only the backend/frontend images, and the all-in-one image when the gate was overridden, build from source. | `docker-publish.yml` |
| `build-android-apk` | release-internal | yes | the release APK assembles, is keystore-signed, `apksigner verify` passes (and matches ANDROID_SIGNING_CERT_SHA256 when set), its versionCode equals the computed value and is > 1, a GH build-provenance attestation + a cosign bundle are produced and attached to the Release, and its sha256 subject is exported for the SLSA generator. | `docker-publish.yml` |
| `apk-provenance` | release-internal | yes | the `slsa-github-generator` reusable workflow signs the APK subject and emits `mycorrhizal-apk.intoto.jsonl` (SLSA build provenance) as a workflow artifact (issue #355). | `docker-publish.yml` |
| `verify-release-assets` | release-internal | yes | attaches `mycorrhizal-apk.intoto.jsonl` and a `SHA256SUMS` manifest to the Release, then asserts the Release carries `app-obtainium-release.apk`, `mycorrhizal-apk.sigstore.json`, `mycorrhizal-apk.intoto.jsonl` and `SHA256SUMS`, and every published image tag resolves in the registry; and that the published all-in-one image digest equals the digest the release gates tested ([#1484](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1484)), failing with an `::error::` otherwise. | `docker-publish.yml` |
| `Test minimum supported versions` | release-tier | yes | the app builds and the suite passes against each declared minimum runtime (COMPAT-02, #473). | `min-version-tests.yml` |
| `Android E2E (emulator, minSdk 26)` | release-tier | yes | the same instrumented suite as `Android E2E (emulator)` passes against the docker-compose.test.yml backend on an API-26 emulator — the declared minSdk floor (COMPAT-02, #473). Runs on push:main, nightly, and dispatch so the floor is exercised pre-tag, not just a number in a build file (issue #927). | `android-tests.yml` |
| `Migration at scale (large dataset)` | release-tier | yes | with MYCORRHIZAL_LARGE_TESTS=1, every supported release migrates to current at ~134x the canonical manifest with row counts and integrity intact (#495). | `migration-tests.yml` |
| `CardDAV real-server E2E` | release-tier | yes | a full round trip against the real reference servers matches the divergence register (#496). | `carddav-e2e.yml` |
| `Constrained-resource chaos` | release-tier | yes | the disk-full / mem-limited / cpu-limited jobs fail closed with no corruption (#498). | `chaos-tests.yml` |
| `Schemathesis API fuzzing` | release-tier | yes | no unhandled 500, no response that violates backend/openapi.yaml, no BOLA finding. | `schemathesis.yml` |
| `Differential E2E (calcard)` | release-tier | yes | our JSContact / iCalendar output matches the pinned reference implementations (#680). | `differential-e2e.yml` |
| `Reference-clients E2E` | release-tier | yes | vdirsyncer round-trips against our server with no data loss (#681); a real DAVx5 client discovers, syncs, and lands the canonical pathological fixture in Android's ContactsContract with no divergence (#917). | `reference-clients-e2e.yml` |
| `ZAP DAST` | release-tier | yes | the baseline scan raises no new high-risk dynamic finding. | `zap-dast.yml` |
| `Clean-install smoke (candidate image)` | release-tier | yes | the candidate release image (pulled by digest, never rebuilt) boots from nothing via the documented compose path: startup ordering (config validated, empty-DB migrations, then serving), the end-to-end register/login/contact/photo/search/export workflow, CORS withheld from a foreign origin, and the blank-`JWT_SECRET_KEY` / empty-`FRONTEND_URL` misconfigurations fail naming the variable (DEPLOY-01, #450; made a release gate by [#1484](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1484) -- it used to run only per-PR on `infra` changes, on `push: main` and nightly). | `deploy-smoke.yml` |
| `OpenSSF Scorecard` | advisory | no | informational supply-chain posture; a score drop is reviewed, never release-blocking. | `scorecard.yml` |
| `CodeQL` | advisory | no | SARIF is uploaded; a new alert is triaged in the Security tab, not release-blocking. | `codeql.yml` |
| `Grype vulnerability scan` | advisory | no | second-opinion CVE scan; the critical/high hard gate is on main + nightly, advisory at release time. | `grype.yml` |
| `TruffleHog secret scan` | advisory | no | verified-secret git-history scan; a hit is investigated immediately but is not a release job. | `trufflehog.yml` |
| `Stryker mutation testing` | advisory | no | frontend/stryker.conf.json's thresholds.break fails the nightly run itself on a mutation-score drop below the ratchet (issue #915); still nightly-only and not release-blocking, so tier stays advisory. | `stryker.yml` |
| `Go mutation testing` | advisory | no | gremlins mutation testing against the safety-critical Go paths (migration/upgrade, backup/restore, delete cascade, import/export, data-integrity invariants — backend/internal/mutationscope.Scopes); each matrix leg's generated config fails the nightly run itself below its recorded threshold (issue #915). Per-leg check names make polling impractical; nightly-only and not release-blocking, so tier stays advisory. | `go-mutation.yml` |
| `Android macrobenchmark` | advisory | no | cold/warm/hot startup and dashboard frame timing trend; continue-on-error by design (emulator variance). | `android-tests.yml` |
| `Reproducible image (all-in-one)` | advisory | no | double-build config + layer digest compare for the linux/amd64 image; continue-on-error until reliably green on main (REL-04, #448). | `reproducibility.yml` |
| `Release dry-run rehearsal` | advisory | no | weekly + on-demand: dispatches release.yml with dry_run:true against the last SupportedReleases entry and fails if that dry run fails; exercises the final-release path's gate battery + fixture regeneration without cutting a release (issue #929). Not release-blocking -- a failure means the automation regressed, triaged like any other advisory gate. | `release-dry-run.yml` |
| `Docker apk pin check` | advisory | no | nightly + on-demand: builds all three Dockerfiles with caching disabled, so a pinned Alpine apk version that upstream has pruned fails here days before it fails a release image build (the #1062/#1131 recurrence guard). Advisory because it is a time-based upstream-drift signal, not a repository change. | `docker-pin-check.yml` |

<!-- release-gates:end -->

## Adding or changing a gate

Edit `.github/release-gates.json` **and** the table above in the same PR (`releasegatecheck`
fails otherwise). A new `release_gate: true` entry must be `per-pr`, have a real `check_run`
context, and be added to the `main-protection` ruleset separately (that config lives in GitHub,
tracked by #508). Moving a suite from `release-tier` to `per-pr` means it is now fast enough to
poll — say so in the PR.
