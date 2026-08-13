# T99 — Drop EB Garamond from the Android app

| | |
|---|---|
| **Platform** | Android |
| **Rating** | 3 |
| **Size** | XS — two call sites, one font family, one test, one attribution entry |
| **Depends on** | Nothing. Partially reverses [T63](87-T63-typography-roles-garamond-mono.md)'s Android port. |
| **Status** | **DONE**, 2026-08-13. `titleLarge` back to sans, both `NavigationDrawerItem` label overrides lose the family, `MycorrhizalFonts.serif` and `eb_garamond.ttf` deleted, `MycorrhizalFonts` import dropped from `MycorrhizalApp.kt`. The Android app now has zero serif usage -- a deliberate divergence from web's T63, recorded in `MycorrhizalFonts`' own doc comment so the next parity audit doesn't re-file it. `ThemeTest`'s `titleLarge` pin was inverted rather than deleted (it now asserts sans), and the font-resource test now asserts `eb_garamond` resolves to 0 while the other three still resolve -- the identical call shape passing for `ibm_plex_sans` in the same test is what proves that assertion isn't vacuous. `Typography`'s class doc keeps T63's reasoning about why `titleLarge` is the only role a brand face can safely occupy, for whoever reaches for one next. **Correction to this ticket's own step 5:** `THIRD_PARTY_SOURCES.md` is *not* edited -- it is a repo-wide attribution file and web still bundles the face (`frontend/public/fonts/EBGaramond-*.woff2`, used by `theme.ts` h5, the nav and the wordmark). Only Android's copy was removed. Not on-device verified -- no device or emulator in this environment. |
| **Source** | Beta testing note, 2026-08-13: *"EB Garamond feels super weird on Android, drop it?"* Decided: drop it. |

## Why this exists

[T63](87-T63-typography-roles-garamond-mono.md) gave web a serif/sans/mono role split and ported it to
Android by mapping web's `h5` → Android's `titleLarge` and web's nav list → the two
`NavigationDrawerItem` label overrides. On web that reads as intended. On a phone, at 22sp, over Material
3's metrics, it does not — a serif display face carries very differently on a small dense screen than in a
1440px page header.

Garamond reaches exactly three places on Android:

- `android/core/ui/.../theme/Theme.kt:164` — `titleLarge` (22sp, Medium), which drives TopAppBar titles and
  the contact-name heading.
- `android/app/src/main/kotlin/com/mycorrhizal/crm/MycorrhizalApp.kt:200` and `:223` — nav-drawer item
  labels, via `MaterialTheme.typography.labelLarge.copy(fontFamily = MycorrhizalFonts.serif)`.
- The drawer header wordmark (`MycorrhizalApp.kt:185-189`) inherits it through `titleLarge`.

The family itself is `MycorrhizalFonts.serif` (`Theme.kt:116-120`), backed by
`android/core/ui/src/main/res/font/eb_garamond.ttf`.

## What to build

1. `Theme.kt:164` — `titleLarge` back to `MycorrhizalFonts.sans`, matching every other role in the block
   (`display*` `:156-158`, `headline*` `:159-161`, `titleMedium/Small` `:165-166`, `body*` `:168-170`,
   `label*` `:172-174`).
2. `MycorrhizalApp.kt:200` and `:223` — drop `fontFamily = MycorrhizalFonts.serif` from both `.copy(...)`
   calls. Note [T100](144-T100-android-drawer-label-size.md) is also editing these two `.copy(...)` calls;
   land them together or sequence them deliberately.
3. Delete `MycorrhizalFonts.serif` (`Theme.kt:116-120`) and `android/core/ui/src/main/res/font/eb_garamond.ttf`.
4. Update `android/core/ui/src/test/kotlin/com/mycorrhizal/crm/ui/theme/ThemeTest.kt`, which currently pins
   `titleLarge` → serif. Change the assertion to sans rather than deleting it — the pin is what stops the
   family silently drifting back.
5. **Leave `THIRD_PARTY_SOURCES.md` alone.** It is a repo-wide attribution file and the web app still
   bundles the face — `frontend/public/fonts/EBGaramond-{Regular,Medium,SemiBold}.woff2`, declared in
   `frontend/public/fonts.css` and used by `frontend/src/theme.ts:85`/`:217` (`h5`), `App.tsx:217` (nav)
   and `App.css:20` (wordmark). Only the Android copy of the font is removed.

## Traps

- **This leaves the Android app with zero serif usage and diverges from web**, which keeps Garamond on `h5`
  and the persistent nav per T63. That divergence is the accepted cost of this ticket, not an oversight —
  record it in the landing note so the next parity audit doesn't file it as a gap.
- Grep for `MycorrhizalFonts.serif` before deleting the property; the two known call sites plus `ThemeTest`
  are the expected hits, but the wordmark and any feature module could have picked it up since.
- Removing a font resource changes the APK's resource table — confirm `assembleDebug` still succeeds and no
  layout references `@font/eb_garamond`.

## Done when

- No reference to `eb_garamond` or `MycorrhizalFonts.serif` remains anywhere under `android/`.
- `ThemeTest` asserts `titleLarge` uses the sans family and passes.
- TopAppBar titles, the contact-name heading and the drawer wordmark all render in IBM Plex Sans.
- `THIRD_PARTY_SOURCES.md` is **unchanged** — web still ships the font.
- `cd android && ./gradlew testDebugUnitTest lintDebug assembleDebug` green.
