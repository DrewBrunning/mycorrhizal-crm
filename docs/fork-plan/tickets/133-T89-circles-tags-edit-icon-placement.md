# T89 — Circles and Tags put their edit pencil at the far right of the header, not next to the label

| | |
|---|---|
| **Platform** | Web |
| **Rating** | 3 — small, but it's the same complaint T74 and T47 already fixed elsewhere |
| **Size** | XS — two `sx` props |
| **Depends on** | Nothing |
| **Status** | **DONE**, 2026-08-13. `ml: 'auto'` -> `ml: 1, flexShrink: 0` on both edit `IconButton`s in `ContactHeader.tsx`, matching the Name field's pencil. One correction to this ticket's own text found while implementing: it claimed the circles/tags pencils are "always visible today" and should stay so -- they are not, they already use the same `opacity: 0` + parent-hover reveal as Name (`'&:hover .edit-icon'`). Only the horizontal placement differed, so only that changed. |
| **Source** | Beta testing note, 2026-08-13: *"Edit still far away from fields (circles, tags) — Name has it next to it, ask fields should just do this."* |

## Why this exists

`frontend/src/components/ContactHeader.tsx` has two different placements for the same affordance.

**The Name field does it right** — `:428-444` (wide) and `:310-326` (compact). The `Typography variant="h5"`
and the edit `IconButton` are immediate flex siblings with `ml: 1, flexShrink: 0`, revealed by the parent's
`'&:hover .edit-icon': { opacity: 1 }` (`:422-424` / `:304-306`). The pencil sits directly right of the
name text, wherever that text ends.

**Circles and Tags do it wrong** — the circles section header row is `:559-568` and the tags one is
`:620-629`. Both are `icon + Typography variant="caption" label + IconButton`, and both give the
`IconButton` **`sx={{ ml: 'auto' }}`** (`:564-567`, `:625-628`), which pushes it to the far right edge of
the header card. On a wide viewport that is several hundred pixels from the "Circles" label it belongs to
and from the chips it edits, which render below at `:606-614`.

This is the same defect class as [T74](118-T74-desktop-field-row-action-distance.md) and
[T47](56-T47-field-action-icons-layout-and-tel-link.md) — an action control separated from its target by
whitespace — in the one place neither ticket touched, because the header is not a field row.

## What to build

Replace `sx={{ ml: 'auto' }}` with the Name field's `sx={{ ml: 1, flexShrink: 0 }}` at both
`ContactHeader.tsx:565` and `:626`.

That is the entire change. Do not restructure the header rows, do not move the chips, and do not add the
hover-reveal behavior — the circles/tags pencils are always visible today and should stay that way (they
sit on a small caption row where a hidden control would be genuinely hard to find, unlike the name, which
is the largest text on the page).

## Traps

- There are **two** edit pencils per section in the file if you grep loosely — the circles/tags ones are
  the `IconButton`s inside the *header rows*, not the per-chip delete icons on the `Chip`s themselves.
- `ml: 'auto'` in a flex row is also what right-aligns other header controls; only the two named lines
  change.

## Done when

- On a contact detail page at 1440px, the pencil for Circles sits immediately right of the word "Circles",
  and the same for Tags — matching the Name field.
- Clicking either still toggles the same `editingCircles` / `editingTags` local state (`:123` and its tag
  counterpart); the edit rows themselves are unchanged.
- At 390px the header still lays out without overflow — verify against
  [T71](115-T71-mobile-circles-tags-add-row-overflow.md)'s wrapping fix, which is in the *edit* row below
  and is not touched here.
- `cd frontend && npx tsc --noEmit && npx vitest run` green.
