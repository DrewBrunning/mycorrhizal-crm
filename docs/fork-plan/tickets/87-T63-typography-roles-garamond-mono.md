# T63 — EB Garamond and IBM Plex Mono are loaded but essentially unused

| | |
|---|---|
| **Rating** | 3 (visual polish/brand identity, not a functional bug) |
| **Size** | S/M — `theme.ts` typography overrides plus scoped component-level exceptions |
| **Depends on** | — |
| **Alpha** | n/a — visual-only, no schema/data change |
| **Source** | User's 2026-08-10 mobile/web content-testing notes: "Mono subheadings and sans content looks good (from mobile testing) - we already have IBM Plex Mono for this on web, we just aren't using it" and "EB Garamond titles look good (except on pop-ups / modals) - the side bar and activity titles only look good as Garamond. Titles on modals and errors don't read well with this though." |

## Why this exists

`public/fonts.css` self-hosts three font families and says why up front: EB Garamond is "branding
only (the wordmark in the top app bar)," IBM Plex Mono is "monospace UI (audit IDs, build version,
webhook tokens)." Checked against the actual app during this session:

- **EB Garamond renders in exactly one place**: the "Mycorrhizal CRM" wordmark
  (`App.tsx:257`, `fontFamily: '"EB Garamond", serif'`). Confirmed live — the contact-page jump nav
  ("Overview," "People," ...) and the `PanelCard` section titles ("Preferences," "Timeline," ...)
  both compute to `"IBM Plex Sans", "Fira Sans", ...` (the theme's global sans default), not
  Garamond.
- **IBM Plex Mono renders nowhere in the live UI.** `index.css` only wires it to the bare `<code>`
  tag, which nothing in the app currently uses for real content — audit IDs, build version, webhook
  tokens (the exact examples `fonts.css`'s own comment names) all render in the default sans, same as
  everything else.

So the user's read from mobile testing — sidebar nav and "activity titles" look better in Garamond,
subheadings look better in Mono, body content stays sans — describes a typography role assignment
that was apparently prototyped/eyeballed on-device but was never wired into `theme.ts`. The one
caveat volunteered unprompted is important: Garamond does *not* read well on dialog/modal titles or
error text, so this isn't "apply Garamond to every heading-shaped element" — it needs a scoped
role split, not a global `typography.h*` override.

## What to build

This needs the same kind of short design/scoping pass as T62 before wiring anything into `theme.ts`
— the open questions are about *which* elements count as each role, not the mechanism:

1. **Confirm the two role assignments against real screens** (mobile, since that's where this was
   observed): which headings are "sidebar nav" (the persistent left nav? the contact-page jump nav?
   both render via different components) and which are "activity titles" (the `PanelCard` section
   headings like "Timeline"/"Preferences"? the individual timeline entries' own subtitle rows in
   `ContactTimeline.tsx`? both?). Get a concrete list of components/variants before touching
   `theme.ts`.
2. **Wire EB Garamond into the confirmed set** — most likely via `theme.ts`'s `typography.h*`/
   `subtitle*` overrides (whichever MUI variant those elements already use), *not* a blanket
   `typography.fontFamily` change, since that would also hit the excluded modal/error case.
3. **Explicitly re-exclude Dialog titles and error/warning text** from whatever variant(s) end up
   Garamond-styled. If step 2 targets a variant like `h5`/`h6` that a `DialogTitle` or an `Alert`
   also happens to use, add a targeted `MuiDialogTitle`/`MuiAlert` `styleOverrides` (or give dialogs
   their own title variant) to force sans there — mirroring how `theme.ts` already carves out
   `MuiDialog`/`MuiPopover`/`MuiMenu` backgrounds as an exception to the base `background.paper`
   token, same pattern, different property.
4. **Wire IBM Plex Mono into confirmed "subheading" elements** — likely `overline`/`caption`-variant
   section labels (e.g. `PreferenceList.tsx`'s "Food & Drink Preferences" section label,
   `ClothingSizesPanel.tsx`'s "Clothing Sizes" label — both already `variant="overline"`) rather than
   real content, per the user's own "mono subheadings, sans content" split.
5. Extend `fonts.css`'s top-of-file comment once the real role list is settled, so it stops saying
   "branding only" / "audit IDs, build version, webhook tokens" if the actual usage ends up broader
   (or keep it accurate to whatever the final scope turns out to be).

## Traps

- **Don't change `typography.fontFamily` (the theme-wide default) or apply Garamond/Mono via a
  top-level `h1`-`h6` override without checking what else renders with that variant.** MUI variants
  get reused in places you don't expect — a `DialogTitle` using `variant="h6"` is exactly the
  collision the user already flagged.
- **EB Garamond only ships Regular/Medium/SemiBold weights** (`fonts.css:6-29`) — check that whatever
  variant gets reassigned doesn't request a weight the font file doesn't have (MUI will silently
  fall back to synthetic-bold or the wrong weight otherwise).
- **This is a visual-consistency change, not a data change** — no backend/migration concerns, but
  still verify in both light and dark mode (`theme.ts` is two independent `createTheme` calls) and at
  mobile widths, since that's where the original observation came from.

## Done when

- A concrete component/variant list for "sidebar nav," "activity titles," and "subheadings" is
  written down (in this file or `95-backlog-and-priorities.md`) before implementation.
- Confirmed live: the assigned elements render in EB Garamond / IBM Plex Mono as scoped; Dialog
  titles and error/warning text render in the default sans, unaffected.
- `cd frontend && npx tsc --noEmit && npx vitest run` green.
- Hand-verified on a real mobile viewport (the environment the original observation came from), not
  just desktop devtools resize.
