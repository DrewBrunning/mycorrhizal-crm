# T35 — Gift tracking gaps: URL field, notes field, full-form add

| | |
|---|---|
| **Status** | **DONE** — see landing note at the bottom of this file |
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

## Landing note

Landed on `feature/t35-gift-tracking-gaps`.

**Backend:** `Gift.URL` (`omitempty,safeurl,max=2000`) and `Gift.Notes` (`omitempty,max=2000`) plus
the matching `GiftInput` fields, and migration `000012` — two additive nullable `TEXT` columns, no
backfill, no existing data touched. `URL` carries an explicit `gorm:"column:url"` tag: GORM's
initialism handling happens to derive `url` correctly here, but an acronym field is precisely the
silent name-mismatch class that shipped broken as `ContactSyncLink.ETag`, so the migration's
spelling is stated rather than inferred. Create and update both assign the new fields; update keeps
its documented full-replace semantics, so omitting `url`/`notes` clears them.

**Frontend:** URL (`type="url"`) and multiline notes fields in `GiftDialog.tsx`, with a client-side
`isSafeUrlString` check mirroring the backend validator so an unsafe scheme is a readable message
rather than a 400. A scheme-less value (`shop.example.com/mug` — what people actually paste) is
normalized to `https://` on save: both validators would have accepted it as-is, and it would then
have rendered as dead text, since only an absolute URI is linkable. `GiftList.tsx` renders a set
URL through T34's tappable-link convention —
`looksLikeAbsoluteUri && isSafeUrlString`, `target="_blank" rel="noopener noreferrer"`, plain text
otherwise — and notes as secondary text under the description. The full-form entry point is an "Add
with details" button beside the quick-add input (the row wraps rather than squeezing the input on a
narrow screen); it opens the same dialog as edit with no gift behind it, so a gift can be recorded
straight as given/received. The quick-add input is untouched, per T20b's own note.

**A bug found while wiring it up:** `handleMarkGivenGift` builds a full-replace `PUT` from the row
in hand, so it would have silently wiped `url`/`notes` on the one-click "mark as given". Both fields
are now carried through, pinned by an e2e round trip.

**Tests:** a real-DB round trip (`TestGift_URLAndNotes_RealMigratedSchema`) covering create, read
back from the physical `url`/`notes` columns, the list surface, update *setting* both fields,
full-replace clearing, and the `safeurl`/`max` rejections — through the real
`ValidateJSONMiddleware`, not the `withValidated` test shim, so the validator genuinely runs. A
migration test asserts that adding the columns preserves pre-existing gift rows (real production
data exists) and that the down migration is clean.
`e2e/gifts.spec.ts` covers the four user-visible paths; frontend unit tests cover the dialog fields,
the safe/unsafe/non-absolute link rendering, and the second entry point. Every new assertion was
hand-verified by breaking the code first — including two Docker rebuild cycles for the e2e ones.

**Review pass:** an Opus review of the branch found no blockers. Fixed from it: the Go suite proved
only that update *clears* url/notes, never that it can set them (a one-line mutation of
`UpdateGift` stayed green — now caught); the scheme-less-URL normalization above; a whitespace-only
url/notes value from a non-dialog writer rendering an empty row; a stale error message left visible
while typing notes; a wrong migration number in a test comment; and a test-coverage asymmetry in
`api/gifts.test.ts`. Deliberately not changed: `isSafeUrlString`'s blocklist (`javascript`/`data`/
`vbscript`/`file`) rather than an http(s) allowlist — that is T34's repo-wide convention, and
diverging for one field is a decision for its own ticket, not a drive-by.

Numbered `000012`, not `000011`: T36 (life event categories) merged to `main` while this was in
flight and took `000011`. Its own migration test already steps exactly one migration, so the two
don't interfere; this ticket's test does the same.
