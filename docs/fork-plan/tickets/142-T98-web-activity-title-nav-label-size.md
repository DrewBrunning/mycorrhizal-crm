# T98 — Activity titles and navigation drawer labels are too small on web

| | |
|---|---|
| **Platform** | Web |
| **Rating** | 3 — readability, on two surfaces used constantly |
| **Size** | XS |
| **Depends on** | Nothing. Adjusts [T63](87-T63-typography-roles-garamond-mono.md)'s landed roles. |
| **Status** | **DONE**, 2026-08-13. All four `subtitle2` title renders in `ContactTimeline.tsx` moved to `subtitle1` together (activity, life event, gift, external activity) so the merged timeline stays internally consistent -- a mixed set of sizes in one list would have been worse than the uniformly-small original. `ActivitiesPage` and `PrepViewPage` activity titles went `body2` -> `subtitle1`, dropping `ActivitiesPage`'s `color: 'text.primary'` override, which only existed to undo `body2`'s themed soil colour. Drawer labels gained `fontSize: '1.0625rem'` in the existing scoped `slotProps.primary.sx`. No shared theme variant was touched, per T63's rule. **`drawerWidth` went 180 -> 256.** Live measurement first showed the bigger label made English's longest nav item ("Circles & Tags", 88px -> 94px against a 91px text slot) newly wrap. The initial fix was a minimal 190, on the reasoning that de/es/fr/it already wrapped at the old 16px and widening far enough for them was out of proportion. **The user rejected that scoping** -- the 180px drawer was too narrow to begin with -- so it went wide enough that no locale wraps in any language, which was never true before. The exact number came from a corrected measurement: a `ListItemButton` spends 48px on padding plus a 56px icon slot, so the text slot is `drawerWidth - 104`, not the `- 88` first assumed. The widest label across all five locales is Spanish's "Registro de auditoría" at 136.16px, needing 240.16 -- so the 240 first proposed misses by a sixth of a pixel and leaves Italian only 1.5px, both on the rounding boundary where a platform font-rendering difference flips them back. 256 gives a 152px slot: 15.8-58px of headroom in every locale. Verified at 1440/1024 (permanent drawer, no horizontal scroll, content area 1184/768) and 375 (drawer out of flow, full-width content). |
| **Source** | Beta testing note, 2026-08-13: *"Activity titles and navigation drawer names need to be bigger on web."* |

## Why this exists

[T63](87-T63-typography-roles-garamond-mono.md) assigned font *families* to typographic roles and
deliberately set **no sizes** — `frontend/src/theme.ts:69-93` (light) and `:208-222` (dark) declare only
`h5: { fontFamily: '"EB Garamond", serif' }` (`:84-86`) and
`overline: { fontFamily: '"IBM Plex Mono", monospace' }` (`:90-92`). The comment at `:78-83` explains why it
refused to retheme `body1`/`body2` globally. So both surfaces below sit at MUI defaults.

**Activity titles** are the smallest text that carries the most meaning:

- `frontend/src/components/ContactTimeline.tsx:208-212` — merged timeline item titles use
  `Typography variant="subtitle2"` (0.875rem). The activity branch is `:210`; life-event, gift and
  external-activity titles use the same variant at `:99`, `:130`, `:168`.
- `frontend/src/ActivitiesPage.tsx:288-296` — the standalone page uses `variant="body2" fontWeight={600}`,
  also 0.875rem, and has to override `color: 'text.primary'` to undo the themed `body2.color` at
  `theme.ts:75-77`.
- `frontend/src/PrepViewPage.tsx:187-190` — `last_activity.title` as `variant="body2"`.

**Drawer labels** are 1rem in a face that renders optically small. `frontend/src/App.tsx:212-215` sets
`slotProps={{ primary: { sx: { fontFamily: '"EB Garamond", serif' } } }}` — **family only, no size** — so
the label falls through to `ListItemText`'s `body1` default. EB Garamond has a smaller x-height than IBM
Plex Sans at the same pixel size, which is why it reads shrunken rather than merely different.

## What to build

1. `ContactTimeline.tsx:208-212` — activity/event titles move from `subtitle2` to `subtitle1` (1rem).
   Keep the existing `fontWeight: 500` and `overflowWrap: 'anywhere'`. Apply the same change to the sibling
   title renders at `:99`, `:130` and `:168` so the merged timeline stays internally consistent — a mixed
   set of sizes in one list is worse than the current uniformly-small one.
2. `ActivitiesPage.tsx:288-296` — `body2` → `subtitle1`, and **drop the now-redundant
   `color: 'text.primary'`**, which exists only to undo `body2`'s themed colour.
3. `PrepViewPage.tsx:187-190` — `body2` → `subtitle1` for the activity title, for the same reason.
4. `App.tsx:212-215` — add `fontSize: '1.0625rem'` alongside the existing family override in the same
   `slotProps.primary.sx`.

Do **not** change the `subtitle2`/`body2`/`body1` variants in `theme.ts`. T63 already established that
retheming shared variants globally is unsafe here, and all four changes above are scoped call sites.

## Traps

- **`drawerWidth = 180`** (`frontend/src/App.tsx:66`). A larger label has less room; check the longest
  string in **all five locales** — German is usually the binding one — and confirm nothing wraps to two
  lines or clips. If it does, widening the drawer is in scope; shrinking the font back is not.
- Both drawers share `drawerContent` (`App.tsx:191-224`), rendered temporary at `:439-449` and permanent at
  `:452-465`, so one change covers mobile and desktop. Verify both.
- `ContactTimeline`'s rows are reused verbatim by `TimelineExplorerDialog.tsx`
  ([T78](122-T78-web-timeline-bounded-view-explorer.md)), so the explorer inherits this change — check it
  still lays out inside the dialog.
- New component test files need an explicit `afterEach(cleanup)` (`/CLAUDE.md` frontend trap #1).

## Done when

- Activity titles render at 1rem on the contact timeline, the timeline explorer dialog, the Activities page
  and the prep view — consistently, with no leftover 0.875rem title in a merged list.
- Drawer labels render visibly larger, on both the permanent and temporary drawers.
- No drawer label wraps or clips at `drawerWidth: 180` in any of the five locales.
- `cd frontend && npx tsc --noEmit && npx vitest run` green.
