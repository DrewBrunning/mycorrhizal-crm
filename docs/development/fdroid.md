# F-Droid submission runbook (issue #1133)

F-Droid builds the Android client itself from a metadata recipe in the
[`fdroiddata`](https://gitlab.com/fdroid/fdroiddata) repository — it does not
accept a pre-built APK. This page is the operator/developer runbook for that
submission. The *why* behind the three distribution variants is ADR
[0022](../adrs/0022-distribution-variants.md).

## The FOSS variant

`:app` has a `distribution` flavor dimension with three flavors:

| Flavor | Purpose | Firebase/GMS |
|---|---|---|
| `obtainium` | gold standard; GitHub Releases + Obtainium | yes |
| `play` | Google Play | yes |
| `foss` | F-Droid | **no** |

F-Droid builds `foss`. Its APK contains no `com.google.firebase` or
`com.google.android.gms` class — the Firebase-backed push code lives in
`android/app/src/fcm/` (included by `obtainium`/`play` only) and the FOSS
source set binds a permanently-unavailable no-op instead. Reminder
notifications on the FOSS build come solely from the WorkManager polling
workers, which are already the fallback everywhere else.

Verify the exclusion locally before submitting:

```bash
cd android
./gradlew :app:assembleFossDebug
unzip -l app/build/outputs/apk/foss/debug/app-foss-debug.apk | grep classes
# then confirm zero hits:
for d in $(unzip -l app/build/outputs/apk/foss/debug/app-foss-debug.apk | grep -o 'classes[0-9]*\.dex'); do
  unzip -p app/build/outputs/apk/foss/debug/app-foss-debug.apk "$d"
done | strings | grep -icE 'com/google/firebase|com/google/android/gms'
```

The same grep over the `obtainium` APK returns thousands of hits; the `foss`
APK must return `0`.

## Dependency compliance

Every remaining dependency is FOSS and resolves from an F-Droid-trusted Maven
repository (`google()`, `mavenCentral()`, and `jitpack.io` in
`android/settings.gradle.kts`):

- `com.github.yalantis:uCrop` (JitPack) — Apache-2.0.
- `net.zetetic:sqlcipher-android` (Maven Central) — BSD-style, with the
  bundled SQLCipher native libraries under their own FOSS licenses.
- `com.google.zxing:core` (Maven Central) — Apache-2.0, a pure-Java QR
  *encoder*; no camera or ML Kit is involved.
- AndroidX / Kotlin / OkHttp / Moshi / Coil / Hilt / Room — Apache-2.0.

No proprietary analytics, ad, or crash-reporting SDK is bundled. The
`foss` flavor's locked dependency graph is `android/app/gradle.lockfile`'s
`fossRelease*` entries; the license-compliance workflow
(`.github/workflows/license-compliance.yml`) scans them like every other
module.

## The recipe

`docs/fdroid/com.mycorrhizal.crm.yml` is the in-repo copy of the fdroiddata
entry. To submit:

1. Fork `https://gitlab.com/fdroid/fdroiddata` on GitLab.
2. Copy `docs/fdroid/com.mycorrhizal.crm.yml` to
   `metadata/com.mycorrhizal.crm.yml` in the fork.
3. Pin `commit` to the **full 40-character hash** of the release being built
   (a tag is not accepted), and set `versionName`/`versionCode` plus
   `CurrentVersion`/`CurrentVersionCode` to match.
4. Add `fastlane/metadata/android/en-US/changelogs/<versionCode>.txt` in the
   main repository for the release (F-Droid picks these up from the source
   checkout, not the recipe).
5. Push the branch and open a merge request against `fdroid/fdroiddata`.

### Versioning is not optional

`versionCode`/`versionName` come from a Gradle property override
(`-PMYCORRHIZAL_VERSION_NAME` / `-PMYCORRHIZAL_VERSION_CODE`), because the
convention plugin's static fallback is `versionCode = 1` / `versionName =
"0.1.0"`. The recipe passes the same values through `gradleprops`; F-Droid
verifies the built manifest against the metadata entry and fails the build on
a mismatch. Without it, every F-Droid release would claim to be versionCode 1
and no update would ever be offered.

## Upstream metadata

F-Droid reads Fastlane metadata from the source repository at
`fastlane/metadata/android/en-US/`:

- `images/icon.png` — 512×512 (a copy of `assets/logo/icon-512.png`).
- `images/featureGraphic.png` — 1024×500.
- `title.txt`, `short_description.txt`, `full_description.txt`.
- `images/phoneScreenshots/` — phone screenshots (add real captures; F-Droid
  lints their absence as a warning).
- `changelogs/<versionCode>.txt` — per release.

When the in-app icon changes, refresh both `assets/logo/icon-512.png` and the
Fastlane copy. The feature graphic is generated from the dark-background mark
(`assets/logo/mark-mycelium-dark-512.png`).

## Post-merge and channel switching

After the MR is merged, wait for the build-server queue and index propagation,
then verify install and update detection through the official F-Droid client.

**F-Droid signs its builds with F-Droid's key, not the project's.** Android
refuses to update an app with an APK signed by a different key, so the F-Droid
APK cannot update over — nor be updated by — the project-signed GitHub
Release/Obtainium APK. Moving between channels requires an uninstall/reinstall.
This is inherent to publishing the same `applicationId` through multiple
stores. The `play` flavor shares this constraint (Google Play App Signing). The
user-facing statement is the [Android app page](../android-app.md).

## The `play` flavor

The `play` flavor omits the call/SMS capture feature entirely (issue #1200):
Google Play restricts `READ_SMS`/`RECEIVE_SMS`/`READ_CALL_LOG` to default
dialer/SMS apps, so `app/src/play/AndroidManifest.xml` removes those
permissions (and the capture components) from its merged manifest,
`app/src/play/res/values/bools.xml` sets `call_sms_tracking_available` false so
Settings hides the toggles and the capture workers are never enqueued, and CI
asserts the built play APK omits the restricted permissions while `obtainium`
keeps them. This affects only `play`; `obtainium` and `foss` are unchanged.
The Google Play submission itself is tracked separately. See ADR 0022.
