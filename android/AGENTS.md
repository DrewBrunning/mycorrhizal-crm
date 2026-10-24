# Android — Kotlin, Compose, Gradle

Read the root `AGENTS.md` first. Android shares the frontend trap IDs (`frontend/AGENTS.md`) for
anything mirrored across clients; the Kotlin-specific rules are here.

## Modules and build

- `android/app` is the application module; `android/core/*` (`data`, `domain`, `model`, `network`,
  `testing`, `ui`) are shared libraries; `android/feature/*` holds screen-level features
  (`contacts`, `settings`, `tracking`, `timeline`, `circles`, …); `android/macrobenchmark` is a
  separate `com.android.test` module; `android/build-logic` holds convention plugins.
- Distribution flavors are `obtainium` (the self-hosted GitHub-release build) and `play` under the
  `distribution` dimension. Proprietary push code lives in the `obtainium`/`play` source sets and the
  shared `fcm` directory; the base code stays flavor-free. Flavor-qualified tasks are the norm —
  `./gradlew :app:connectedObtainiumDebugAndroidTest` for instrumented tests.
- `detekt` is the Kotlin lint gate: `cd android && ./gradlew detekt` on any Kotlin change. detekt 2.x
  supports the JDK 25 build JVM, so no JDK downgrade is needed; CI runs the same task in
  `android-tests.yml`'s `Android (Gradle)` job. There is no Gradle step in the local pre-commit hook
  (too slow to launch a build per commit).
- `lint` is the Android resource/manifest/framework gate and treats **warnings** as errors (issue
  #1902): `./gradlew lintDebug :app:lintObtainiumDebug :app:lintFossDebug`. The shared config is
  `android/lint.xml`, where every `ignore` carries a reason (dependency freshness is Renovate's).
  There is deliberately no `lint-baseline.xml`: a new exemption needs a reasoned per-check `ignore`
  and, when it is deferred work, a linked follow-up issue.

## Instrumented E2E

The suite in `android/app/src/androidTest` drives the real app against the real
`docker-compose.test.yml` backend — login → list → detail → edit, favorites, archive/delete + audit
undo, deep links, local mode, and the rest. Emulator:
`cd android && ./gradlew :app:connectedObtainiumDebugAndroidTest`. Physical device (Pixel 8a):
`adb reverse tcp:7300 tcp:7300` first, then the same task with
`-Pandroid.testInstrumentationRunnerArguments.serverUrl=http://127.0.0.1:7300`. Full runbook in
`README-developer.md`; CI runs it via the `android-e2e` job in `android-tests.yml`. An Android
ticket with an E2E gate should point here, not at a manual on-device checklist.

## Macrobenchmark

`android/macrobenchmark` measures cold/warm/hot startup (`StartupBenchmark`) and dashboard
render/scroll frame timing (`DashboardRenderBenchmark`, logged into the same
`docker-compose.test.yml` backend the E2E suite uses). It runs against the app's `benchmark` build
type (release R8 config, debug-signed, profileable). **Not a gate** — the `android-macrobenchmark`
job is `continue-on-error` and asserts no timing threshold (emulator variance); it is a trend signal
and a local tool (`./gradlew :macrobenchmark:connectedCheck`; the emulator-guard `suppressErrors`
list is baked into `macrobenchmark/build.gradle.kts`). No Baseline Profile is shipped yet.

## Client mirrors

Enums and registries on the wire are hardcoded mirrors of the backend `oneof` validators (frontend
trap #4) — there is no dynamic type-list endpoint. Adding a token backend-side means updating the
Kotlin copy (`core/model/.../registry/`, `ContactFieldKey`, etc.) by hand, with a comment noting the
mirror. The network models in `core/model/network` are pinned against the shared contract fixtures;
the generated TS types do not cover Kotlin, so contract changes need a deliberate Android review.
