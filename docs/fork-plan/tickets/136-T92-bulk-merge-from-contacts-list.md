# T92 — Merge isn't in the Contacts bulk-actions list

| | |
|---|---|
| **Platform** | Web |
| **Rating** | 4 — the reported blocker after a large import |
| **Size** | M |
| **Depends on** | [T93](137-T93-duplicate-scan-endpoint-and-review.md) for the *suggested-pairs* half. The select-two-and-merge half depends on nothing. |
| **Status** | **DONE** (2026-08-14 — see the landing note below) |
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

## Landing note, 2026-08-14

Shipped steps 1–4; step 5 needed nothing (T93's "Review duplicates" surface already walks its pairs
through `MergeContactsDialog`'s pair mode — the exact lift this ticket deferred to it). T107 landed
first, so the reachability-vs-destruction trap is moot: the loser's attachments/preferences/cadence/
external identities all re-point now.

What actually got built:

- **`BulkActionsBar` gains a Merge button** (`onMerge` prop), enabled only at `selectedCount === 2`
  and disabled otherwise with a tooltip (`bulk.mergeSelectTwoHint`), via a `Tooltip`-wrapped `<span>`
  because a disabled MUI Button never fires pointer events. Two new keys (`bulk.merge`,
  `bulk.mergeSelectTwoHint`) in all five locales.
- **`ContactsPage` resolves the pair from the loaded page, not a lookup round trip.** Selection is
  keyed by `Contact.VCardUID`; the rows already carry `Contact.ID`, which is what
  `POST /contacts/merge` wants. The handler is defensive — it re-filters the loaded `contacts` by the
  selection set and refuses unless exactly two resolve, even though the button only enables at two.
- **After a successful merge: clear the selection and refetch list + circles + tags.** The loser is
  gone; a stale selection pointing at it (and stale circle/tag chips on the survivor) is the T94/T95
  bug class this ticket's step 4 explicitly names.
- The **detail-page entry point is untouched** and re-verified by `contactMerge.spec.ts`.

Tests:

- `BulkActionsBar.test.tsx`: disabled at 1 and 3, enabled + reports at exactly 2, tooltip text on
  hover, disabled while busy. The jsdom tooltip assertion needed the mouse event on the `Tooltip`'s
  own `<span>` (MUI only opens when the pointer enters the wrapper element, not a disabled
  descendant).
- `ContactsPage.test.tsx`: select-two → dialog opens in pair mode showing both "Keep …" candidates →
  resolve → commit called with the right IDs → selection cleared and `getContacts` refetched. Plus a
  cross-page variant (Alice on page one + Carol on page two via "load more") proving the resolution
  reads the merged loaded array, and a stale-selection variant (below).
- New `e2e/bulkMerge.spec.ts`: select one (Merge disabled + tooltip) → two (enabled) → three
  (disabled again — merge is strictly pairwise) → back to two → dialog offers both keeper
  candidates with the list-first row as default → **Swap toggles the checked keeper and back** →
  the distinct first names produce a **real firstname conflict the user resolves via the keeper
  radio** (deliberately not the no-conflict fast path) → commit → dialog closes, selection empty,
  loser gone from the list, keeper's GET ok and loser's polled to 404 (the T93 WAL-read-snapshot
  caveat). Full e2e suite green against the Docker test stack.

Both unit tests were hand-verified by breaking them first: disabling the enable-gate made the
single/three+ tests fail, and stubbing out the `setMergePair` call made the ContactsPage test fail —
each restored after confirming the failure.

One process note: the "disabled hint" e2e assertion originally clicked the disabled button to surface
the tooltip; Playwright's actionability check rightly refused. Hovering is the correct gesture for a
disabled element — a `hover({ force: true })` is needed because the opened tooltip then sits over the
button and would block a normal actionability re-check.

## Review pass, 2026-08-14

Self-review after the landing found one real bug and two coverage gaps; all fixed in this pass:

- **An enabled Merge button could silently do nothing.** The selection is keyed by `VCardUID` and
  deliberately survives a sort change (T77) — but the sort's refetch replaces the loaded page, which
  may carry fewer than both selected rows. `handleBulkMerge`'s resolution guard then bailed silently:
  a visible, enabled Merge button that no-ops on click, the exact failure class this repo avoids. It
  now `window.alert`s (`bulk.mergeSelectionStale`, ×5 locales) and clears the stale selection, so the
  user recovers instead of staring at a dead button. Pinned by a unit test that reproduces the
  sequence (select across pages → sort → refetch drops the row → click Merge): it fails against the
  silent version and passes against the fix. Note the T77 "sort doesn't clear the selection" decision
  was left intact — the merge path now *handles* a selection it can't resolve rather than the sort
  changing the contract.
- **Cross-page merge resolution was untested.** The whole point of uid-keyed selection is surviving
  pagination, and the resolution reads the merged loaded array — but the first test only merged two
  rows from page one. Added a unit test: Alice (page one) + Carol (page two) → dialog shows both →
  commit called with `(1, 3)`.
- **e2e covered the one/two gate but not three-plus or the swap.** The ticket's "Done when" lists
  one-or-three-plus disabled explicitly. The spec now seeds a third contact: 3 selected → Merge
  disabled, back to 2 → enabled. And it drives the Swap control in the real dialog (default keeper
  checked → Swap → the other checked → Swap back), proving the "lets the keeper be swapped"
  requirement end-to-end rather than only in the dialog's own unit tests.
