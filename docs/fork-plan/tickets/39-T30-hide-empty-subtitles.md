# T30 — Hide section subtitles when the section has no visible fields

| | |
|---|---|
| **Rating** | 3 — small but a real, visible annoyance |
| **Size** | S |
| **Depends on** | — |
| **Alpha** | n/a — real data exists; this is display-only, no schema change |
| **Source** | v0.2.0-alpha real-world testing |

## Why this exists

Sections on the contact detail page (and the contact list entry) show a subtitle even when every
field under that subtitle is hidden — e.g. "Card metadata" (`contact.card.metadata` /
`en.json:397`) still renders when the user has hidden every field it would otherwise introduce.
An empty section with a label but nothing under it reads as broken.

This was reported twice independently in the same testing pass (once as "Card Metadata showing
when all fields are hidden," once as the general "contact field subtitles still show when all
fields in a category are hidden") — same bug, one ticket.

## What to build

Audit every section/subsection header on `ContactDetailPage.tsx` / `ContactInformation.tsx` (and
the list-entry equivalent, if it has its own subtitled sections) that is driven by
`contactFields.ts`'s field-visibility toggles. For each, the subtitle should only render when at
least one field in that section is both present on the contact **and** enabled in field
visibility settings — the same "would this section render anything" check the section body
already has to make to decide whether to render its own contents, just hoisted one level up so
the header shares it instead of computing it twice.

Likely the cleanest fix is a small shared helper (e.g. `hasVisibleFields(section, contact,
fieldVisibility)`) used by both the header and the body, rather than duplicating the visibility
logic in two places — a good candidate for a shared hook if the same check needs to run in the
list-entry variant too.

## Traps

- Don't just hide on "all fields empty" — a field can be non-empty on the contact but *disabled*
  in field visibility settings, and vice versa. The check needs both conditions.
- Make sure the fix doesn't flicker: if visibility state loads asynchronously, avoid a render
  where the subtitle briefly shows before the fields do.

## Done when

- With every field in a section hidden (via field visibility settings) or empty, that section's
  subtitle does not render, on both the contact detail page and any other surface with the same
  pattern (e.g. list entries, `AddContactDialog` if it shares the pattern).
- With at least one field visible, the subtitle still renders normally.
- `npx tsc --noEmit` clean, `npx vitest run` green — add a component test asserting the subtitle
  is absent when all fields in a section are toggled off.
