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

## Landing note

**2026-08-11 (landed):** Role list settled via two parallel Explore audits plus user answers:

- **"The side bar"** = the app's persistent left nav only (`App.tsx`) — the contact-detail page's own
  jump nav (Overview/People/Timeline/...) was considered and explicitly excluded.
- **"Activity titles"** = literally each page's own top-level heading ("Dashboard," "Contacts,"
  "Search," etc, and the contact-detail page's own heading — the contact's name). `PanelCard` section
  titles ("Preferences"/"Timeline"), individual timeline-entry titles, and the standalone Activities
  list's `activity.title` were all considered and excluded — they stay plain sans.

**The mechanism ended up different from this ticket's own guess**, and turned out to matter a lot:

- `h5` is used *exclusively* for page-level headings app-wide (confirmed zero live collisions — MUI's
  `DialogTitle` defaults to `h6`, not `h5`) — safe for a clean `theme.ts typography.h5` override, both
  light and dark blocks.
- `overline` (the Mono subheading candidate) is likewise used exclusively for the section-label role
  ("Food & Drink Preferences," "Clothing Sizes," 5 more) — safe for a clean `theme.ts
  typography.overline` override, both blocks.
- The persistent nav renders via a bare `ListItemText primary=` with **no** variant — it implicitly
  falls through to MUI's global `body1` default, which is reused 19+ times explicitly plus is the
  fallback for every unstyled Typography/ListItemText/MenuItem app-wide. **Not safe to theme
  globally** — wired instead via `slotProps.primary.sx` scoped to just this one `App.tsx` nav list
  (using MUI v7's current `slotProps` API, not the deprecated `primaryTypographyProps`, per explicit
  instruction to prefer current APIs).
- Every other candidate the ticket floated (`h6`, `subtitle1`, `subtitle2`, `body2`, `Button`
  typography) turned out to have real collisions if reassigned globally — `h6` hits all ~40 live
  `DialogTitle`s, `subtitle2` hits a `color="error"` site inside a dialog, `subtitle1`/`body2`/`button`
  are each reused dozens-to-hundreds of times for unrelated things. Moot here since none of those
  variants ended up in scope, but worth remembering if a future ticket wants to extend either font's
  reach.

No `MuiDialogTitle`/`MuiAlert` re-exclusion override was needed — `h5` and `overline` never touch a
variant `DialogTitle`/`Alert` use, so there was nothing to exclude. `AlertTitle` in particular can
never pick up `h5`: it renders via `Typography` with no `variant` prop passed, so it always falls
through to MUI's own default (`body1`), regardless of anything in this app's theme.

Hand-verified live (browser preview, real dev-DB data, both light and dark mode, desktop docked drawer
and mobile hamburger drawer): every page heading and the contact-name heading render in EB Garamond;
the nav list (both drawer variants) renders in EB Garamond; "Food & Drink Preferences" and "Clothing
Sizes" render in IBM Plex Mono; the "Add Preference" `DialogTitle` stays plain sans, confirming no
leak. `npx tsc --noEmit` and `npx vitest run` (622/622) green.

`public/fonts.css`'s top-of-file comment updated to describe the real final roles instead of the
stale "branding only" / "audit IDs, build version, webhook tokens" description (the latter was never
actually true — Mono rendered nowhere live before this ticket).

**2026-08-11 (follow-up).** User feedback after hand-testing: the `overline`-only Mono placement was
too easy to miss (it only appears on the two niche section labels named above, which don't show up
on a contact with no preferences/clothing sizes set — plausibly why it read as "not seeing Mono
anywhere"). The actually-expected role was per-field captions on the contact detail page — "Birthday,"
"Phone," "Address," etc — to contrast against the IBM Plex Sans field values below them. Checked
first: `caption` (the variant those labels use) is reused 69 times app-wide, including error text and
dialog content, so — same reasoning as the `h5`/`overline` decision above — not safe for a blanket
`theme.ts typography.caption` override. Fixed with a component-scoped `sx` override instead, on the
two shared components that render every field-row caption on the contact page:
`EditableField.tsx:60` (single-value fields) and `EditableArrayField.tsx:83` (multi-value fields,
phone/address/email/etc) — together these cover all 23 field rows in `ContactInformation.tsx`. The
`overline` section-label treatment was kept, not reverted — it's still a correct, harmless "subheading"
instance, just not sufficient on its own. Hand-verified live: "Birthday"/"Phone"/"Address"/"Email"/etc
render in Mono, the field values themselves stay sans, and unrelated captions elsewhere (e.g.
`ContactHeader.tsx`'s "Circles"/"Tags" labels) are confirmed untouched. `npx tsc --noEmit` and
`npx vitest run` (622/622) still green.
