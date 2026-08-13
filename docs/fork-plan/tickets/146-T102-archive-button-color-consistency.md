# T102 — The Archive button is a different colour depending on where you find it

| | |
|---|---|
| **Platform** | Web |
| **Rating** | 3 |
| **Size** | XS — three colour props |
| **Depends on** | Nothing. Fills a gap [T62](86-T62-badge-and-button-color-system.md) left. |
| **Status** | **DONE**, 2026-08-13. `color="warning"` off Archive, `color="success"` off Unarchive, and the archived `Chip` from `warning` to `default` -- so all four archive surfaces (wide header, compact menu, bulk bar, list badge) now match. **The rule, for the next reversible action:** reversible state changes take the neutral outlined treatment; `warning`/`error` are reserved for destructive or irreversible ones like delete. That is the gap [T62](86-T62-badge-and-button-color-system.md) left, now filled. |
| **Source** | Beta testing note, 2026-08-13: *"Archive button has color inconsistent (web) — sometimes yellow, sometimes green depending on where it is."* |

## Why this exists

The same conceptual action renders four different ways:

| Surface | File:line | Colour |
|---|---|---|
| Contact header (wide), **Archive** | `frontend/src/components/ContactHeader.tsx:524-534` | `color="warning"` — chanterelle, yellow |
| Contact header (wide), **Unarchive** | `frontend/src/components/ContactHeader.tsx:448-458` | `color="success"` — moss, green |
| Contact header (compact <md), both | `ContactHeader.tsx:392-397` / `:347-353` | none — inherits menu text colour |
| Bulk actions bar, both | `frontend/src/components/BulkActionsBar.tsx:157-162` | none — default primary/mycelium outline |

So it is yellow on a wide contact page, green once archived, uncoloured on a phone, and brand green on the
list. Because you rarely see the pair together, it reads as random rather than as a deliberate
archive-is-warning / unarchive-is-success split — which is itself the wrong semantics: archiving is
reversible and not a warning condition.

The archived **badge** is inconsistent the same way: `Chip color="warning"` at `ContactHeader.tsx:289-296`
versus `Chip color="default"` at `frontend/src/ContactsPage.tsx:407-414`.

[T62](86-T62-badge-and-button-color-system.md) established the colour system — chips go neutral unless the
colour carries genuine semantic meaning, "Add X" actions go brand green, decorative field glyphs stay
`text.secondary`. It never issued a rule for **reversible destructive-adjacent** actions like archive,
which is the gap this sits in.

## What to build

Standardise on `variant="outlined"` with **no explicit `color`** for both Archive and Unarchive, matching
`BulkActionsBar.tsx:157-162` — the one pair that is already internally consistent.

1. `ContactHeader.tsx:524-534` — drop `color="warning"`.
2. `ContactHeader.tsx:448-458` — drop `color="success"`.
3. `ContactHeader.tsx:289-296` — archived `Chip` from `color="warning"` to `color="default"`, matching
   `ContactsPage.tsx:407-414` and T62's chips-go-neutral rule.

The compact overflow-menu items (`:392-397`, `:347-353`) already inherit menu text colour and stay as they
are — a `MenuItem` is a different control, and colouring one entry in a menu is exactly what T62 moved away
from.

**Record the rule in the ticket's landing note**, so the next reversible action doesn't have to rediscover
it: reversible state changes (archive, unarchive) are neutral outlined buttons. `color="warning"` and
`color="error"` are reserved for genuinely destructive or irreversible actions — delete, purge, revoke.

## Traps

- Do not fold this into a theme-level `MuiButton` override. Three call sites is not a pattern worth
  centralising, and the theme's `MuiChip` block (`frontend/src/theme.ts:261-280`) is already carrying T62's
  dark-mode scoping fix — adding button variants there risks re-flattening it.
- Check the archived chip in **dark mode**. T62's landing fixed a dark-mode chip-flattening bug specifically
  for `color="default"`; the chip changing to `default` is the case that fix exists for, so verify it looks
  right rather than assuming.
- New component test files need an explicit `afterEach(cleanup)` (`/CLAUDE.md` frontend trap #1).

## Done when

- Archive and Unarchive render identically on the contact header, on the bulk actions bar, and on both at
  1440px and 390px.
- The archived badge is neutral on both the contact detail page and the contacts list, in light and dark
  mode.
- No `color="warning"` or `color="success"` remains on an archive-related control.
- The neutral-for-reversible rule is written into the landing note.
- `cd frontend && npx tsc --noEmit && npx vitest run` green.
