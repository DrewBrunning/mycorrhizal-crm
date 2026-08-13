# T97 — Android status bar colour is inverted in dark mode

| | |
|---|---|
| **Platform** | Android |
| **Rating** | 4 — visible on every screen, every launch, for any dark-mode user |
| **Size** | M — three places set bar appearance and none is theme-aware |
| **Depends on** | Nothing. Related to [T106](150-T106-android-splash-screen-dark-mode.md) (same missing `values-night/`). |
| **Status** | **TO BE DONE** |
| **Source** | Beta testing note, 2026-08-13: *"Status bar color is inverted in dark mode (needs to be dark on the now light background and be light when the dark navigation drawer is open)."* |

## Why this exists

Four independent places set status-bar appearance, and **not one of them reads the current theme**.

| Where | What it does |
|---|---|
| `android/app/src/main/res/values/themes.xml:3` | Parent theme is `android:Theme.Material.Light.NoActionBar` — Light-only, and there is no `values-night` counterpart anywhere under `android/`. |
| `android/app/src/main/res/values/themes.xml:12-14` | Hardcodes `android:statusBarColor` `#3E543E` (light `mycelium`), `windowLightStatusBar` `false`, `navigationBarColor` `#FAF5EA` (light `bone`). |
| `android/app/src/main/res/values-v27/themes.xml:7-8` | `windowLightNavigationBar=true` — correct only over a light nav bar. |
| `android/app/src/main/kotlin/com/mycorrhizal/crm/MainActivity.kt:17` | `enableEdgeToEdge()` with **no arguments**. `darkTheme = isSystemInDarkTheme()` is read at `:19` and passed only to `MycorrhizalTheme` at `:20` — never to bar styling. |
| `MycorrhizalApp.kt:166-178` | `LaunchedEffect(drawerState.isOpen)` → drawer open: `MycorrhizalColors.parchment` + `isAppearanceLightStatusBars = true`; closed: `MycorrhizalColors.mycelium` + `false`. **Both constants are the light-palette values** (`Theme.kt:26`, `:29`), never `parchmentDark`/`myceliumDark` (`:40`, `:42`). |
| `android/feature/contacts/.../ContactDetailScreen.kt:192-215` | A per-scroll override doing the same thing with `MycorrhizalColors.bone` (light) and `mycelium`, plus a `DisposableEffect` restore at `:209-215`. |

So in dark mode the drawer sheet renders `surfaceContainerLow` = `parchmentDark` (`Theme.kt:98`) — a dark
surface — while `MycorrhizalApp.kt:168-171` paints the status bar light `parchment` with dark icons. The
contact-detail top state does the same with `bone` over a `boneDark` background. That is the reported
inversion, in both directions.

## What to build

1. **Stop reading `MycorrhizalColors` directly.** `Theme.kt:24-50` exposes light and dark as separate flat
   properties with no `@Composable` accessor, so there is no "current scheme" indirection to fix centrally
   — which is the actual root cause. Replace every bar-colour read with the corresponding
   `MaterialTheme.colorScheme.*` value (`surfaceContainerLow` for the drawer state, `primary` for the
   collapsed app-bar state, `surface`/`background` for the contact-detail top state), which is already
   theme-aware via `MycorrhizalTheme`.
2. **Derive icon appearance from the theme, not from a literal.** `isAppearanceLightStatusBars` must be
   `!darkTheme` in the base case, inverted only where the underlying surface genuinely inverts (drawer
   open over a light app bar). Thread `darkTheme` down from `MainActivity.kt:19` rather than re-deriving
   `isSystemInDarkTheme()` at each site.
3. **Pass the scrims to `enableEdgeToEdge`.** `MainActivity.kt:17` should call it with explicit
   `SystemBarStyle` values built from `darkTheme`, so the initial frame is right before any
   `LaunchedEffect` runs.
4. **Add `values-night/themes.xml`** with the dark-palette `statusBarColor`/`navigationBarColor` and
   `windowLightStatusBar=true`, and a `values-night-v27/themes.xml` for `windowLightNavigationBar=false`.
   The base theme parent should also stop being `Light`-only — use a `DayNight` parent.

## Traps

- **`window.statusBarColor` is deprecated and a no-op on Android 15+.** `targetSdk = 35`
  (`android/build-logic/.../AndroidConfig.kt:29`) plus `enableEdgeToEdge()` means the runtime colour
  assignments at `MycorrhizalApp.kt:168`/`:173` and `ContactDetailScreen.kt:197`/`:211` may already do
  nothing on newer devices, leaving only `isAppearanceLightStatusBars` effective. **Verify which of the two
  actually still applies before designing around either** — on a modern device the fix may be entirely
  about icon appearance plus letting the composable's own background show through.
- There are **three** effects fighting over the same property: the drawer effect, the contact-detail scroll
  effect, and its `DisposableEffect` restore. They already race — the restore at `:209-215` hardcodes the
  closed-drawer state, so disposing the contact screen while the drawer is open leaves the bar wrong.
  Whatever the fix, make one owner.
- `LocalDrawerOpen` (`android/core/ui/.../LocalDrawerOpen.kt:10`, provided at `MycorrhizalApp.kt:180`,
  consumed at `ContactDetailScreen.kt:190`) already exists to coordinate these two — use it rather than
  adding a second channel.

## Done when

- In dark mode, with the drawer closed, the status bar matches the app bar and its icons are legible.
- In dark mode, opening the drawer keeps the icons legible against the dark drawer sheet.
- Both of the above still hold in light mode, unchanged from today.
- Scrolling the contact detail screen to collapse and re-expand its header, in both themes, leaves the bar
  correct; closing that screen with the drawer open does too.
- Verified on a real device in both themes — the failure mode is entirely visual and unit tests cannot see
  it.
- `cd android && ./gradlew testDebugUnitTest lintDebug assembleDebug` green.
