# T88 — The contact-info two-column layout collapses around pronouns, How We Met and Additional Information

| | |
|---|---|
| **Platform** | Web |
| **Rating** | 3 — cosmetic, but it undoes part of what T74 was for |
| **Size** | S — one component, four `FullSpanField` decisions plus a grid track change |
| **Depends on** | Nothing. [T74](118-T74-desktop-field-row-action-distance.md) built the grid this adjusts. |
| **Status** | **TO BE DONE** |
| **Source** | Beta testing note, 2026-08-13: *"Two column layout breaks down on web (pronouns, How We Met, Additional Information)."* |

## Why this exists

[T74](118-T74-desktop-field-row-action-distance.md) turned `ContactInformation.tsx`'s field list into a
CSS grid — `gridTemplateColumns: { xs: '1fr', lg: 'repeat(2, 1fr)' }` at
`frontend/src/components/ContactInformation.tsx:542-550` — with a `FullSpanField` wrapper
(`:117-119`, `gridColumn: { lg: '1 / -1' }`) for fields that genuinely need the width.

Five fields opted into `FullSpanField`, and three of them are exactly the ones the report names:

| Field | Line | Section |
|---|---|---|
| `speakToAs` (pronouns) | `:786-805` | `genderAndPronouns` |
| `work_information` | `:871-888` | — |
| `cardNotes` | `:892-906` | `notes` |
| `how_we_met` | `:908-925` | `notes` |
| `contact_information` ("Additional Information") | `:927-944` | `notes` |

Two distinct failures follow, and both are what the user is seeing:

1. **A hole beside Gender.** A `grid-column: 1 / -1` item cannot be placed mid-row, and the grid has no
   `dense` flow. In `genderAndPronouns` the order is heading (full-span) → `gender` (column 1, `:766`) →
   `speakToAs` (full-span, `:786`), so the browser leaves column 2 next to Gender empty and starts
   pronouns on the following row. Any section with an odd number of normal fields before a
   `FullSpanField` does the same.
2. **The whole Notes section renders single-column.** `cardNotes`, `how_we_met` and
   `contact_information` are three consecutive full-span fields under a full-span heading (`:890`), so
   at `lg`+ the entire bottom of the card is one column. That is "the two-column layout stops working,"
   observed exactly where it stops.

There is also a latent third problem the report doesn't name: the tracks are `1fr`, which is
`minmax(auto, 1fr)` — a long unbreakable value can force a column past 50%. `EditableField.tsx:71` and
`EditableArrayField.tsx:82` each set `minWidth: 0`, which mitigates it, but the grid itself has no floor.

## What to build

The decisions below are made; do not re-derive them.

1. **`speakToAs` is full-span only while editing.** Its display renderer (`renderSpeakToAs`, `:484-504`)
   emits one short line, so it has no reason to span in read mode; its editor (`SpeakToAsEditor`) has
   several sub-fields and does. Make the `FullSpanField` wrapper at `:787` conditional on the field's
   editing state. That removes the hole beside Gender in the common (non-editing) case.
2. **`how_we_met` and `contact_information` stop being full-span.** Drop the `FullSpanField` wrappers at
   `:909` and `:928` so the two sit side by side as ordinary grid items. Both are `multiline`
   `EditableField`s; multiline text wraps fine in a ~530px column, and pairing them is what the report
   is asking for.
3. **`cardNotes` and `work_information` stay full-span.** Card notes are free-form prose of arbitrary
   length and work information is the same shape; both read badly at half width. Leave `:872` and `:893`
   alone.
4. **Give the tracks a floor.** Change `:545` to
   `gridTemplateColumns: { xs: '1fr', lg: 'repeat(2, minmax(0, 1fr))' }`. Apply the same change to
   `SectionGroup`'s `twoColumn` branch in `frontend/src/ContactDetailPage.tsx:144-165`, which has the
   identical `repeat(2, 1fr)`.

## Traps

- **Do not reach for `grid-auto-flow: dense`.** It fills holes by pulling later small items backwards,
  which reorders fields out of their section order — and the contact page's jump nav
  ([T45](54-T45-contact-jump-nav-mobile-dropdown.md)) plus `contactSectionVisibility.ts` both assume
  source order is display order.
- **The metadata collapse toggle hard-codes its span inline** at `:964` rather than using
  `FullSpanField`. Leave it — it genuinely spans, and the inline form is deliberate.
- Section membership lives in `frontend/src/contactSectionVisibility.ts:22` (`genderAndPronouns`) and
  `:24` (`notes`). Item 2 does not change which section a field belongs to, only how wide it renders.
- Custom fields (`CustomFieldValueRow`, `:948-955`) are never full-span regardless of type, so a
  long-text custom field already renders at half width. That asymmetry is pre-existing and out of scope
  here — note it, don't fix it.
- New component test files need an explicit `afterEach(cleanup)` (`/CLAUDE.md` frontend trap #1).

## Done when

- At 1440px, Gender and pronouns sit side by side in read mode, with no empty cell between them.
- At 1440px, "How We Met" and "Additional Information" sit side by side.
- Card notes and work information still span both columns.
- At 1024px and 390px the card is byte-identically single-column — the `lg` breakpoint is untouched.
- Entering edit mode on pronouns widens that field to full span and does not reflow the fields above it
  into a different order.
- `frontend/src/components/ContactInformation.test.tsx` gains cases for the pronouns edit/read span
  difference and for the how-we-met/additional-info pairing; the existing full-span assertions at
  `:426-480` are updated, not deleted.
- `cd frontend && npx tsc --noEmit && npx vitest run` green.
