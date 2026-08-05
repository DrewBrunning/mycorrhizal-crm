# T35 — Gift tracking gaps: URL field, notes field, full-form add

| | |
|---|---|
| **Rating** | 3 — same rating as the T20b feature it extends |
| **Size** | S |
| **Depends on** | [T20b](28-T20b-gift-tracking.md) (done) |
| **Alpha** | n/a — real data exists. Two new nullable columns on `gifts` (additive migration) plus a frontend change; no data loss risk |
| **Source** | v0.2.0-alpha real-world testing |

## Why this exists

Three gaps found using the shipped T20b gift-tracking feature for real, all against the same
`Gift` entity — grouped as one ticket per the user's own instruction.

## What to build

1. **URL field on gift ideas.** `Gift` has no field for "here's a link to the thing" — useful for
   an idea captured as "she mentioned she liked this specific item" with a product page to
   reference later. Add `URL string` to `models/gift.go` (`validate:"omitempty,safeurl,max=2000"`
   — reuse the existing `safeurl` validator), a migration adding the nullable column, and a field
   in `GiftDialog.tsx` (type="url", rendered as a tappable link in `GiftList.tsx` once set —
   consistent with [T34](43-T34-contact-field-linking.md)'s "raw links are tappable" convention if
   that ticket has landed by the time this one does).

2. **Notes field.** `Gift.Description` is the core "what it is" field; there's no separate place
   for additional context (sizing notes, where you saw it, a reminder like "check if they still
   want this before buying"). Add `Notes string` (`validate:"omitempty,max=2000"`, same shape as
   other free-text note fields in this repo), migration, and a multiline field in `GiftDialog.tsx`
   below the description.

3. **Full-form add, not idea-only quick-add.** `GiftList.tsx`'s inline `onAdd(description)` always
   creates a `status: idea` record — there's no way to directly add a gift that's already been
   given or received without creating it as an idea first and then editing it to change status.
   Add a second entry point alongside the existing quick-add input that opens the full
   `GiftDialog` (already has a status selector) for adding a gift with any starting status. Keep
   the quick-add input as-is for the idea-capture case — per T20b's own note, that low-friction
   path is the point of the feature and shouldn't be removed, just supplemented.

## Traps

- `GiftDialog.tsx` already has a status `<TextField select>` — the "full form" path in item 3 is
  mostly about *exposing* the existing dialog from a new "Add gift" affordance on `GiftList.tsx`,
  not building new status-handling logic.
- Keep `URL` and `Notes` in the same T26 soft-delete/cascade posture as the rest of `Gift` — no
  new entity, no new cascade-list changes needed, just new columns.
- Migration: hand-written SQL up/down pair adding two nullable `TEXT` columns to `gifts`, per
  `CLAUDE.md` (no `AutoMigrate`).

## Done when

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` green, with a real-DB round-trip
  test covering the two new fields.
- `npx tsc --noEmit` clean, `npx vitest run` green.
- Hand-verified: add a gift idea with a URL and notes, confirm both persist and display; use the
  new full-form entry point to add a gift directly as "given" without going through "idea" first.
- All 5 locale files have real translations for the new field labels.
