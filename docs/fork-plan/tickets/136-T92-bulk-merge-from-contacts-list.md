# T92 — Merge isn't in the Contacts bulk-actions list

| | |
|---|---|
| **Platform** | Web |
| **Rating** | 4 — the reported blocker after a large import |
| **Size** | M |
| **Depends on** | [T93](137-T93-duplicate-scan-endpoint-and-review.md) for the *suggested-pairs* half. The select-two-and-merge half depends on nothing. |
| **Status** | **TO BE DONE** |
| **Source** | Beta testing note, 2026-08-13: *"How can I bulk merge? Import created a lot of duplicates and merge isn't in the bulk actions list on Contacts."* |

## Why this exists

Merge is reachable from exactly one place: a contact's own detail page — `ContactHeader.tsx:362-366`
(overflow menu) and `:471-478` (inline button) → `ContactDetailPage.tsx:1372`. So cleaning up 40 import
duplicates means 40 round trips through two contact pages each.

The Contacts list has a bulk-actions bar (`frontend/src/components/BulkActionsBar.tsx`, rendered at
`frontend/src/ContactsPage.tsx:353-372`) with seven actions, enumerated at
`frontend/src/api/bulkOperations.ts:7-16`: `add_circle`, `remove_circle`, `add_tag`, `remove_tag`,
`archive`, `unarchive`, `delete`. Backend dispatch is `runBulkContactAction`
(`backend/controllers/bulk_operation_controller.go:166-224`). Merge is absent from all three surfaces —
no `BULK_ACTIONS` entry, no `onMerge` prop in `BulkActionsBarProps` (`:17-32`), no case in the dispatcher.

## What to build

**Merge is not a bulk operation in the sense the other seven are.** Those apply one verb to N independent
rows. Merge is inherently pairwise, needs a keeper chosen per pair, and can surface field conflicts that
require a human answer (`ComputeContactMergeResolution`,
`backend/services/contact_merge_service.go:50-76`). Do **not** add a `merge` case to
`runBulkContactAction` — a blind N-way merge with auto-resolved conflicts is a data-destroying operation on
real production data.

Build it as a guided flow instead:

1. **Entry point.** Add a "Merge" button to `BulkActionsBar`, enabled only when **exactly two** contacts are
   selected. Anything else shows it disabled with a tooltip explaining the constraint.
2. **Reuse the existing dialog.** Open `MergeContactsDialog` pre-populated with both contacts rather than
   with an empty search picker. That requires lifting its current "the viewed contact is always the loser"
   assumption (`MergeContactsDialog.tsx:37-41`, `loadPreview(contact.ID, currentContactId)` at `:90`) into
   an explicit keeper/loser choice with a swap control — which the list flow needs anyway, since neither
   selected row is privileged.
3. **Selection is by `VCardUID`, the merge endpoint takes numeric IDs.** `ContactsPage`'s selection set is
   keyed by uid; `POST /contacts/merge` wants `keep_id`/`merge_id` (`backend/routes/routes.go:72-73`,
   `models.ContactMergeRequest`). The rows already in hand carry `ContactSummary.ID`
   (`backend/models/contact_summary.go:31`), so resolve from the loaded page rather than adding a lookup
   round trip.
4. **After a successful merge, refresh the list and clear the selection.** The loser is gone; a stale
   selection pointing at it is the same class of bug as
   [T94](138-T94-merge-dialog-stays-open.md)/[T95](139-T95-merge-keeper-shows-stale-circles.md).
5. **Once [T93](137-T93-duplicate-scan-endpoint-and-review.md) lands**, add a second entry point: a
   "Review duplicates" button on the Contacts page that walks T93's candidate pairs through this same
   dialog one at a time. That is the actual answer to "I have a lot of duplicates" — not selecting pairs by
   hand. Ship step 1–4 first; this step is additive.

## Traps

- **Merge destroys the loser's attachments, preferences, cadence policy and external identities** — see
  [T107](151-T107-merge-destroys-attachments-and-more.md). Making merge easier to reach makes that bug
  easier to hit. T107 should land first, or this ticket must surface those counts in the preview.
- `MergeContactsDialog` currently hard-codes its direction. Changing that touches the detail-page entry
  point too — verify the existing flow still merges the *viewed* contact away by default, since that is
  what users of that surface expect.
- New component test files need an explicit `afterEach(cleanup)` (`/CLAUDE.md` frontend trap #1).

## Done when

- Selecting exactly two contacts on the Contacts page enables a Merge action; selecting one or three-plus
  disables it with an explanation.
- The dialog opens with both contacts filled in, lets the keeper be swapped, and shows the same conflict
  resolution UI as the detail-page flow.
- After a merge the list refetches, the loser is gone, and the selection is empty.
- The detail-page merge entry point still behaves as it does today.
- New strings translated in all five locales.
- `cd frontend && npx tsc --noEmit && npx vitest run` green, plus a Playwright spec driving select-two →
  merge → list-refreshed.
