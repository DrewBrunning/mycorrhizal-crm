# T74 — Field action buttons (edit/call/copy) sit too far from their field on wide desktop screens

| | |
|---|---|
| **Platform** | Web |
| **Rating** | 3 — cosmetic/layout, but touches every field on the most-viewed card of the most-used page |
| **Size** | M — a layout change to three shared components, gated to one breakpoint |
| **Depends on** | [T47](56-T47-field-action-icons-layout-and-tel-link.md) (done — the right-edge alignment this ticket is the desktop-widescreen downside of) |
| **Status** | **DONE**, 2026-08-12. Both levels landed exactly as designed. **Level 1** (`ContactInformation.tsx`): the field-list wrapper became a CSS grid (`display: grid; gridTemplateColumns: { xs: '1fr', lg: 'repeat(2, 1fr)' }; columnGap: 3; rowGap: 2; alignItems: 'start'`), replacing the `<Stack spacing={2}>` — `SectionHeading` and a new `FullSpanField` wrapper (multiline fields, SpeakToAs, card notes, the metadata toggle) get `gridColumn: { lg: '1 / -1' }` so they still stretch full width; every other field narrows for free, per the ticket's own claim that `EditableField`/`EditableArrayField` need no changes. **Level 2** (`ContactDetailPage.tsx`): `SectionGroup` gained an opt-in `twoColumn` prop (only `people`/`timeline`/`cadence` pass it — `overview`/`gifts`/`external-links`/`attachments` are untouched, exactly per the design's explicit list) and `PanelCard` gained `fullWidth` (used on Connections' graph and the merged timeline). **Measured, not eyeballed**: at 1440px the Gender field's action cluster moved from **1078.8px → 498.8px** from its value (a **53.8% reduction**, row width 1136px → 556px, both confirmed via `getBoundingClientRect()` before/after a `git stash`/`pop` cycle). Verified `people`/`timeline`/`cadence` render 2-up at 1440px and 1280px (`Relationships` alone + `Connections` full-width row; `Timeline` full-width + `Life Events`/`Conversation Agenda` side by side; `Cadence`/`Reminders` side by side) and collapse to one column at 1024px and 390px, byte-identical to pre-change at both — no horizontal overflow at 390px. `overview`'s two cards (`ContactInformation`, `Preferences`) confirmed still full-width, unchanged. Jump-nav anchor scroll re-verified clear of the AppBar/sticky nav. Confirmed no scroll-jump entering edit mode on a narrowed field. `cd frontend && npx tsc --noEmit && npx vitest run` green (632/632). |
| **Source** | Testing notes, 2026-08-11: "on desktop web the edit button is on the far right. On a wide screen this makes it hard to find/tap because it's so far from what you can see you're editing. The layout makes a lot of sense on narrow columns like mobile, but is unwieldy on desktop web. This also applies to actions like call and copy." User flagged this explicitly needed a design pass, not a quick fix. |

## Why this exists

Confirmed structural, with the numbers computed from source rather than eyeballed:

- The page column is capped at 1200px (`ContactDetailPage.tsx:1305`:
  `Box sx={{ maxWidth: 1200, mx: 'auto', mt: 1, px: 2, pb: 2 }}`), minus 32px of page padding = a
  1168px `Card`, minus `CardContent`'s 16px each side = a **1136px field row**.
- `EditableField.tsx:55-117` and `EditableArrayField.tsx:73-98` both lay a row out as
  `icon → flexGrow:1 label/value box → action cluster`, with the cluster anchored at the row's far
  right edge. The value starts ~34px in (after the icon); the actions end at 1136px. **So a short
  value and its own edit button can sit ~1100px apart.**
- This is a direct side effect of [T47](56-T47-field-action-icons-layout-and-tel-link.md), which
  moved action icons from "crowding the value text" to "grouped at the row's right edge" — the right
  call on mobile, where "right edge" and "near the value" are nearly the same place. On a wide
  desktop card they are not.
- [T31](40-T31-contact-tabs-info-architecture.md) is the other half: it replaced the tab strip with
  one single-column scrollable page of full-width `PanelCard`s. A wide viewport currently buys a
  wider single column, not more columns.

## The design — decided 2026-08-11

Direction chosen: **use the reclaimed desktop width for a genuine two-column layout**, rather than
capping widths and leaving the space empty.

### The finding that shapes it

Two-column *cards* alone does not fix this ticket's own complaint. Mapping the actual structure:

| Section | `PanelCard`s |
|---|---|
| `overview` | **1** (`ContactInformation`) + Preferences |
| `people` | 2 |
| `timeline` | 3 |
| `cadence` | 2 |
| `gifts` / `external-links` / `attachments` | 1 each |

`ContactInformation` renders **a single `<Card>`** (`ContactInformation.tsx:524`) containing all
~30 field groups — including all eleven `render*List` families T47 enumerated. That one card is the
worst offender by a wide margin, and no card-level two-column scheme narrows it. Worse, pairing it
with the short Preferences card in a 2-up grid would leave thousands of pixels of empty right
column beside it.

So the fix has to happen at **two levels**:

### Level 1 — two-column field rows inside `ContactInformation` (the actual fix)

At `lg`+ only, lay `ContactInformation`'s field groups out in a two-column CSS grid inside its
existing card:

```
display: grid
gridTemplateColumns: { xs: '1fr', lg: 'repeat(2, 1fr)' }
columnGap: 3, alignItems: 'start'
```

Each field row becomes ~530px wide, so its action cluster lands ~530px from the value instead of
~1136px — a >50% reduction, achieved by *using* the width rather than discarding it. `EditableField`
and `EditableArrayField` need **no changes at all**: they already flex to their parent, so a
narrower parent is the whole mechanism. That is what keeps this an `M` and not an `L`.

Use `gridAutoFlow: 'row'` (the default) so reading order stays left-to-right, top-to-bottom and the
current logical field order is preserved. Do **not** use CSS multi-column (`columnCount`), which
flows column-major and would reorder every field on the page.

Multi-line and wide fields (card notes, `SpeakToAs`, anything with an inline editor) should opt into
`gridColumn: '1 / -1'` to span both columns.

### Level 2 — two-column `PanelCard`s per section (the space the user asked to reclaim)

`SectionGroup` (`ContactDetailPage.tsx:137-146`) gains the same responsive grid, so the `people`,
`timeline`, and `cadence` sections lay their cards 2-up at `lg`+. **Grid inside each section, not
masonry across the page** — that keeps every section a full-width block, so T31's anchor targets and
`scrollMarginTop: 112` keep working exactly as they do now, with no jump-nav changes.

`PanelCard` gains a `fullWidth?: boolean` that sets `gridColumn: '1 / -1'`, for cards that need the
width: `ContactInformation`, the merged timeline, and `ConnectionsPanel`'s graph are the obvious
ones. Single-card sections (`gifts`, `external-links`, `attachments`) are unaffected — a 1-item grid
renders identically to today.

`alignItems: 'start'` keeps cards at their natural heights rather than stretching the short one to
match its neighbor.

### What is deliberately unchanged

- **Everything below `lg` (1200px)** — mobile and tablet are byte-identical to today. T47's
  narrow-viewport behavior is the reason this bug doesn't occur there, and it must not be disturbed.
- **T47's grouping** — actions stay grouped and right-aligned within their row. They just have a
  much shorter row to sit at the end of. This ticket does not re-open T47.
- **T31's single-scroll IA** — still one scrollable page, still the same seven anchor sections, still
  the same jump nav.
- **The 1200px page cap.** Two columns at 1200px give ~530px each, which is a good measure for a
  field row. Widening the container to 1400 would make the columns 630px and put the actions
  *further* away again — the opposite of the goal.

## Traps

- **Don't re-apply T47's pre-fix "crowd the text" layout as the desktop fix.** That is exactly what
  T47 was filed to get away from; it would re-open T47 rather than resolve this ticket's actual
  complaint, which is distance on *wide* screens specifically.
- **Cover both call-site families.** T47 touched both `EditableField`/`EditableArrayField` and
  `ContactInformation.tsx`'s eleven `render*List` functions. Level 1's grid must wrap the field
  groups such that *all* of them are affected, or some fields narrow and others don't,
  inconsistently. Audit against T47's own enumeration: phone, email, links/IMPP, address, online
  services, personal info, notes, languages, speak-to-as, anniversaries, keywords.
- **Grid items with `minWidth: 0`.** A flex/grid child defaults to `min-width: auto`, so a long
  unbroken value (an email, a URL) can force its column wider than `1fr` and blow out the layout.
  `EditableArrayField` already sets `minWidth: 0` on its content box; `EditableField.tsx:59` uses
  bare `flex: 1` and does not. Add it.
- **Editing state changes row height.** A field switching into edit mode grows; with
  `alignItems: 'start'` its neighbor stays put, which is correct. Verify it doesn't cause a
  reflow that scrolls the page out from under the user mid-edit.

## Done when

- At a 1440px-wide viewport, a field's action cluster sits roughly half the previous distance from
  its value — verify by measuring, not by eye, and record the before/after numbers in the landing
  note.
- `ContactInformation`'s fields render in two columns at `lg`+ in their existing logical order, and
  in one column below `lg`.
- The `people`, `timeline`, and `cadence` sections render their cards 2-up at `lg`+; single-card
  sections are visually unchanged.
- Jump-nav anchors still scroll each section's title clear of the AppBar and sticky nav.
- Hand-verified at 1440px, 1280px, 1024px (single column), and a 390px phone viewport — the last two
  must be indistinguishable from today.
- Every field family in T47's enumeration confirmed to have narrowed, not just the ones going through
  `EditableField`.
- `cd frontend && npx tsc --noEmit && npx vitest run` green.
