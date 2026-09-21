# ADR 0022: Distribution variants — obtainium, foss, and play

- **Status:** accepted
- **Date:** 2026-09-21
- **Depends on:** issue #152 (FCM push client), issue #238 (Android E2E), issue #263 (macrobenchmark),
  issue #721 (call/SMS tracking permissions), the F-Droid inclusion policy
- **Implements:** issue #1133 (F-Droid submission, and the broader app-store coverage it needs)

## Context

The Android client has exactly one build, shipped as a signed APK attached to each GitHub Release and
updated on-device by [Obtainium](https://obtainium.imranr.dev/), plus a manual
`android-apk-build.yml` dispatch for testers. That build compiles Firebase Cloud Messaging (FCM) as a
latency fast-path for reminder notifications; the WorkManager polling workers are the fallback and the
sole path on de-Googled devices.

That single build cannot be submitted to F-Droid. F-Droid's [Inclusion Policy](https://f-droid.org/en/docs/Inclusion_Policy/)
forbids proprietary dependencies and explicitly names Google Play Services and Firebase; an app that
fails to build without Play Services "will receive a rejection". It also cannot be submitted to Google
Play as-is for a different reason (see the permissions note below).

The forces pulling the build apart:

| Concern | Who cares |
|---|---|
| No Firebase/GMS in the APK | F-Droid (blocking) |
| Firebase push latency | Play/Obtainium users who have it configured |
| `READ_SMS`/`READ_CALL_LOG` tracking permissions | Google Play restricts them to default dialer/SMS apps |
| One signing key per install | Android refuses to update an APK signed by a different key |

## Decision

Build **three product flavors** from one source tree, on a single `distribution` flavor dimension
declared in `:app` only. The library modules stay flavor-free; all proprietary push code lives in the
app's per-flavor source sets, so no other module ever sees a Firebase type.

| Flavor | Channel | Firebase/GMS | Tracking permissions | Who signs it |
|---|---|---|---|---|
| `obtainium` | GitHub Releases, updated by Obtainium | yes (optional at runtime) | present | the project's release keystore |
| `play` | Google Play | yes | **omitted** (issue #1200) | Google Play App Signing / project keystore |
| `foss` | F-Droid | **no** | present | F-Droid's key |

`obtainium` is the gold standard: it is what the release workflow builds, what the instrumented E2E
suite runs against, and what coverage is measured from. `play` exists so Play-only additions (in-app
updates, Play Integrity) have a home where they cannot leak into the FOSS build. `foss` is the only
variant F-Droid's build server is ever asked to build.

### How the split is implemented

- `:feature:tracking` keeps the **interfaces** (`FcmAvailability`, `FcmTokenSource`) and the
  Firebase-free machinery (`DeviceRegistrationManager`, `DeviceRegistrationViewModel`,
  `PushNotificationDispatcher`, `PushMessageParser`, the polling workers). It has **no** Firebase
  dependency, so every variant inherits a FOSS-clean module graph.
- `:app`'s `src/fcm/` source set — included by `obtainium` and `play` only — holds the Firebase-backed
  implementations (`FirebaseFcmAvailability`, `FirebaseFcmTokenSource`, `MyFirebaseMessagingService`)
  and their Hilt module (`FcmPushModule`). The FCM service is declared in the **main** manifest and
  removed for `foss` by `src/foss/AndroidManifest.xml` (a declared service whose superclass is absent
  from that variant's classpath must not survive the merge).
- `:app`'s `src/foss/` holds `FossPushModule`: an availability seam that is permanently `false` and a
  token source that throws if ever reached (it is not — `DeviceRegistrationManager` checks availability
  first). The FOSS APK therefore carries no Firebase or GMS class, and reminder push is delivered
  exclusively by the WorkManager polling workers.
- The `com.google.gms.google-services` Gradle plugin is still applied only when a real
  `google-services.json` is present, which it never is in F-Droid's checkout, and it has no per-flavor
  switch — a developer's local Play build processes all flavors, a shipped FOSS build never does.

### Versioning

`versionCode`/`versionName` stay owned by the convention plugin
(`MycorrhizalAndroidApplicationPlugin`), overridden for releases with
`-PMYCORRHIZAL_VERSION_NAME`/`-PMYCORRHIZAL_VERSION_CODE`. The F-Droid recipe passes the same two
properties through `gradleprops`, because F-Droid verifies the built manifest's version against the
metadata entry — without them every F-Droid release would ship as the plugin's static
`versionCode = 1`.

### Signing and channel switching

Each channel signs with a different key (the project's keystore for Obtainium, F-Droid's for F-Droid,
Google's for Play), and Android refuses to update an installed app with an APK signed by another key.
**Switching channels requires uninstall/reinstall.** This is inherent to distributing the same
`applicationId` through multiple stores; it is not a bug, and it is documented for users on the
[Android app page](../android-app.md). All three
flavors deliberately keep `applicationId = com.mycorrhizal.crm` so the F-Droid recipe and any future
Play listing target the canonical package.

## Consequences

- CI's flavor-qualified tasks (`:app:testObtainiumDebugUnitTest`, `:app:assembleFossDebug`, …) are the
  new names; the unflavored `testDebugUnitTest`/`assembleDebug` now match libraries only. See
  `docs/development/testing.md` and `README-developer.md`.
- Gradle dependency locking (`issue #942`) moved from the exact `release*Classpath` names to a suffix
  match so each flavor's release graph is locked independently — the `foss` lock carries no Firebase,
  the `obtainium`/`play` ones do.
- Offline JaCoCo instrumentation and the aggregated report pick exactly one coverage variant per module
  (`obtainiumDebug` where flavored, else `debug`). Kotlin mangles `internal` members with the
  variant-specific module name (`performEnroll$app_fossDebug`), so flavor class trees are not
  interchangeable and must not be merged into one coverage report.
- **The `play` flavor omits the call/SMS capture feature (issue #1200).** Google Play restricts
  `READ_SMS`/`RECEIVE_SMS`/`READ_CALL_LOG` to default dialer/SMS apps, so `play` removes those
  permissions and the capture components/permissions from its merged manifest
  (`app/src/play/AndroidManifest.xml`), hides the capture toggles in Settings, and never enqueues the
  capture workers — all driven by the `call_sms_tracking_available` resource it overrides to false.
  `obtainium` and `foss` keep the full feature. The Google Play *submission* itself is tracked
  separately (AAB, Play App Signing, listing, policy review).

## Alternatives considered

- **Remove Firebase entirely.** Simplest and fully FOSS, but it deletes a shipped feature (the push
  latency fast-path, issue #152) for the Obtainium/Play users who already have it and changes the
  backend's device-registration behavior. Rejected: the flavor split preserves it at the cost of one
  source-set boundary.
- **Per-module flavors.** Applying the dimension to every module (or a shared `feature:push` module
  that only `play`/`obtainium` depend on) spreads three variants across the whole build graph for a
  boundary that only the app module needs. Rejected: keeps `:feature:tracking` Firebase-free and
  confines the variant fan-out to `:app`.
- **A `foss`-only build with no Play/Obtainium flavors.** Would satisfy F-Droid and the issue's minimum,
  but leaves no place for Play-only code and forces a future Play effort to redo the split. Rejected as
  short-sighted given the explicit goal of broad store coverage.
