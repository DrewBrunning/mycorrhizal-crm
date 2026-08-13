# T106 — The Android splash screen is always light

| | |
|---|---|
| **Platform** | Android |
| **Rating** | 2 — one second per launch, but it's the first thing a dark-mode user sees |
| **Size** | S |
| **Depends on** | Nothing. Shares the missing `values-night/` with [T97](141-T97-android-status-bar-dark-mode.md); land them together. |
| **Status** | **TO BE DONE** |
| **Source** | Beta testing note, 2026-08-13: *"Splash screen for Android is always light mode, doesn't respect dark mode."* |

## Why this exists

**There is no splash screen implementation at all.** No `androidx.core:core-splashscreen` dependency
anywhere in `android/gradle/libs.versions.toml` or any `build.gradle.kts`; no `installSplashScreen()` call
— `MainActivity.kt:15-17` goes straight from `super.onCreate` to `enableEdgeToEdge()`; no
`windowSplashScreenBackground`, `windowSplashScreenAnimatedIcon`, `postSplashScreenTheme` or
`windowBackground` attribute anywhere.

What the user sees is the **Android 12+ system-generated splash**, which derives entirely from the app
theme declared at `AndroidManifest.xml:45` (`@style/Theme.Mycorrhizal`) and the launcher icon. Both are
light-only:

- `android/app/src/main/res/values/themes.xml:3` — parent is `android:Theme.Material.Light.NoActionBar`.
- `android/app/src/main/res/values/colors.xml:3` — launcher icon background `#FAF5EA` (light `bone`),
  consumed by `res/mipmap-anydpi-v26/ic_launcher.xml`.

And there is **no `values-night/` directory anywhere under `android/`** — the only qualified resource dirs
are `values-v27` and `core/ui/res/values-{de,es,fr,it}` for strings. So nothing can vary by theme, for the
splash or anything else.

## What to build

1. **Add `androidx.core:core-splashscreen`** to `android/gradle/libs.versions.toml` and
   `android/app/build.gradle.kts`. `minSdk = 26` (`android/build-logic/.../AndroidConfig.kt:28`), so the
   compat library is the portable route — the platform API alone only covers 31+.
2. **Declare a splash theme** with `windowSplashScreenBackground` and `windowSplashScreenAnimatedIcon`, and
   `postSplashScreenTheme` pointing at `@style/Theme.Mycorrhizal`. Point `AndroidManifest.xml:45` at the
   splash theme instead.
3. **Call `installSplashScreen()` in `MainActivity.onCreate` before `super.onCreate`** — that ordering is
   required by the library, not stylistic.
4. **Add `values-night/themes.xml`** with the dark-palette background, and `values-night/colors.xml` for
   the launcher icon background. Make the base theme parent a `DayNight` variant rather than `Light`.
5. Add `values-v31/themes.xml` and `values-night-v31/themes.xml` if the platform API's attributes need to
   differ from the compat library's on 31+.

## Traps

- **[T97](141-T97-android-status-bar-dark-mode.md) needs the same `values-night/themes.xml`.** Two tickets
  creating the same file will collide; land T97 first, or land both in one change. This ticket assumes the
  file may already exist.
- The launcher icon background (`colors.xml:3`) is also the adaptive-icon background on the home screen, so
  a `values-night` override changes the icon too. That is usually desirable, but it is a second visible
  change — confirm it looks right rather than only checking the splash.
- The system splash is drawn by the OS before any of your code runs; only the theme can influence it. There
  is nothing to test in a unit test — this one is verified on a device, in both themes, or not at all.

## Done when

- Launching the app in dark mode shows a dark splash that matches the app's dark background, with no white
  flash between splash and first frame.
- Light mode is unchanged from today.
- Verified on a real device by cold-launching in each theme — the failure mode is a sub-second visual and
  nothing else can see it.
- `cd android && ./gradlew testDebugUnitTest lintDebug assembleDebug` green.
