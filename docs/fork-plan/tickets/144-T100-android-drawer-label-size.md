# T100 — Android navigation drawer labels are too small

| | |
|---|---|
| **Platform** | Android |
| **Rating** | 3 |
| **Size** | XS — one property on two `.copy(...)` calls |
| **Depends on** | Nothing. Shares its two edit sites with [T99](143-T99-android-drop-eb-garamond.md). |
| **Status** | **DONE**, 2026-08-13. `fontSize = 16.sp` added to both scoped `labelLarge.copy(...)` calls in `MycorrhizalApp.kt`; `Theme.kt`'s `labelLarge` untouched, and `ThemeTest`'s comment on that role now names the size rule as well as the family rule. Landed in the same commit as [T99](143-T99-android-drop-eb-garamond.md), which edits the same two calls. Not on-device verified -- no device or emulator in this environment, so the "buttons are unchanged by eye" and per-locale wrapping checks are still outstanding. |
| **Source** | Beta testing note, 2026-08-13: *"Navigation drawer names need to be bigger on Android."* The web half is [T98](142-T98-web-activity-title-nav-label-size.md). |

## Why this exists

Drawer item labels use `MaterialTheme.typography.labelLarge` — **14sp**, Medium, 20sp line height
(`android/core/ui/.../theme/Theme.kt:172`) — at
`android/app/src/main/kotlin/com/mycorrhizal/crm/MycorrhizalApp.kt:197-202` (primary destinations) and
`:220-225` (secondary destinations).

14sp is Material's *label* size, meant for chips and button text, not for a primary navigation list. M3's
own `NavigationDrawerItem` guidance is `labelLarge`, but the drawer here is the app's only global
navigation and carries ten destinations (`MycorrhizalApp.kt:98-103` primary, `:106-114` secondary) against
a 22sp wordmark at `:185-189` — the contrast is what makes them read as undersized.

## What to build

Add `fontSize = 16.sp` to the existing scoped `.copy(...)` at `MycorrhizalApp.kt:200` and `:223`. Line
height scales with the style, so no separate adjustment is needed.

## Traps

- **Do not change `labelLarge` in `Theme.kt:172`.** It is also M3's default for `Button` and `Snackbar`
  labels — the comment at `MycorrhizalApp.kt:193-196` says so explicitly — so a global bump resizes every
  button in the app. The two scoped `.copy(...)` calls already exist precisely to avoid that; use them.
- `ThemeTest.kt` pins `labelLarge`'s family. It should be unaffected by a call-site-only change, but check
  it still passes.
- [T99](143-T99-android-drop-eb-garamond.md) edits the same two `.copy(...)` calls to remove the serif
  family. Land them in one change or sequence them deliberately — after both, the calls carry a size and no
  family override.
- Ten destinations at 16sp in five locales: confirm the longest German label doesn't wrap inside the
  drawer sheet's width.

## Done when

- Drawer item labels render at 16sp on both the primary and secondary destination groups.
- Button and Snackbar text is unchanged — verify by eye on a screen with both, not just by reading the diff.
- No drawer label wraps or truncates in any of the five locales.
- `cd android && ./gradlew testDebugUnitTest lintDebug assembleDebug` green.
