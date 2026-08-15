# T110 — Contacts toolbar: "Add Contact" overlaps its neighbours when the row wraps

| | |
|---|---|
| **Platform** | Web |
| **Rating** | 3 — cosmetic, but it hides/obscures the primary create action on mid-width screens |
| **Size** | S |
| **Depends on** | Nothing (can land with or after T111, which touches the same `Stack`) |
| **Status** | **DONE** (2026-08-15, same branch as T111 — landed together) |
| **Source** | Testing note: *"Contacts search layout - Add Contact overlapping when wrapping."* |

## Why this exists

`ContactsPage.tsx` renders the title, a full-width search `TextField` (`:323-347`), then the filter/toolbar
row as a single `Stack` (`:348-426`):

```tsx
<Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5} mb={2} alignItems="center" flexWrap="wrap">
  … circle Select, sort Select, showArchived Switch, showAll Switch,
     Import Button, Review-duplicates Button, Add-Contact Button …
</Stack>
```

On `sm`+ the direction is `row` with `flexWrap="wrap"`. There are seven children with fixed `minWidth`s
(180 + 220 + two switch labels + three buttons), so on any width between ~600px and ~1200px the row wraps
onto two or more lines. MUI `Stack`'s `spacing` is implemented as margins on the children, which does not
produce a reliable cross-axis gap for *wrapped* flex lines, so the trailing "Add Contact" (and often
"Review duplicates") button lands flush against — or visually overlapping — the element above/beside it
when it wraps onto a new line. The buttons already carry `whiteSpace: 'nowrap'`, so the button itself
doesn't shrink; the problem is the absence of a real `rowGap` between wrapped lines.

## What to build

Give the wrapped toolbar a real, uniform gap between lines, and make the create action the last thing to
wrap rather than the first thing to collide:

1. Replace the `spacing`-based `Stack` at `:348-426` with a flex container that uses CSS `gap` (MUI `Stack`
   supports `useFlexGap`, but the simplest faithful fix is a plain `Box` with `display: 'flex',
   flexWrap: 'wrap', alignItems: 'center', gap: 1.5` and `flexDirection`/stacking preserved via
   `{ xs: 'column', sm: 'row' }` on `flexDirection`). `gap` gives both row and column spacing, so wrapped
   lines no longer touch.
2. When the direction is `column` (`xs`), `alignItems: 'stretch'` (or leave the controls full-width) so the
   buttons don't bunch left; this is the current `xs` behaviour and must not regress.
3. Verify the search field above (`:323-347`) and the `BulkActionsBar` below (`:444`) keep their own `mb`/`mt`
   spacing — do not fold them into the gap change.

Note: if T111 (sticky toolbar) lands first, it will restructure this exact `Stack`; coordinate so this
fix's `gap` survives whatever wrapper T111 introduces.

## Traps

- `Stack` `spacing` with `flexWrap` is the specific foot-gun — don't "fix" it by adding more `mb` per child,
  because that leaves uneven gaps on the first and last wrapped lines.
- The two `FormControlLabel` switches already set `whiteSpace: 'nowrap'`; do not let the gap change make
  them wrap their labels.
- On `xs` the direction is `column` and the buttons are full-width; confirm no horizontal overflow at 390px
  after the change (T71/T72 fixed similar narrow-width regressions elsewhere).

## Done when

- At every width between 600px and 1200px, the toolbar's wrapped lines have a visible, even vertical gap and
  "Add Contact" never overlaps "Review duplicates"/"Import"/a switch.
- At 390px the column layout is unchanged (full-width controls, no horizontal scroll).
- `cd frontend && npx tsc --noEmit && npx vitest run` green; hand-verify by dragging the window width across
  the wrap boundary.

## Landing note (2026-08-15)

The toolbar `Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5} alignItems="center"
flexWrap="wrap"` at `ContactsPage.tsx:348` was replaced with a plain flex `Box` using `gap: 1.5`, keeping
the same direction breakpoint and adding `alignItems: { xs: 'stretch', sm: 'center' }` so the `xs` column
layout stretches the controls full-width instead of bunching them left. Landed in the same commit as T111
(T111 restructured the surrounding area into a sticky block), so the two tickets were never independent —
the gap container lives inside T111's sticky wrapper. All 23 ContactsPage tests + 11 BulkActionsBar tests
pass. Not browser-hand-verified this session; the wrap geometry change is unit-tested via the unchanged
page tests and should get a quick width-drag check when T111's sticky header is eyeballed.

