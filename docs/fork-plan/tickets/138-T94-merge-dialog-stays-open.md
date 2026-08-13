# T94 — The merge dialog stays open after a successful merge

| | |
|---|---|
| **Platform** | Web |
| **Rating** | 4 — leaves the UI in a state where a second click merges the wrong pair |
| **Size** | XS |
| **Depends on** | Nothing. Same root cause as [T95](139-T95-merge-keeper-shows-stale-circles.md); fix both together. |
| **Status** | **DONE**, 2026-08-13. `handleCommit` captures the keeper id, then calls `handleClose()` (reusing the existing reset, not duplicating it) before `onMerged`; the failure branch deliberately does not close. `ContactDetailPage`'s `onMerged` also sets `mergeDialogOpen` false itself, so the page owns its own dialog state for the second entry point [T92](136-T92-bulk-merge-from-contacts-list.md) will add. Two new tests in `MergeContactsDialog.test.tsx` (success closes + reports the keeper; failure leaves it open with the selection intact); the success one hand-verified to fail with "expected spy to be called at least once" when the `handleClose()` call is removed. Landed with [T95](139-T95-merge-keeper-shows-stale-circles.md), same commit -- one root cause. |
| **Source** | Beta testing note, 2026-08-13: *"Close merge modal after merging succeeds on web (remains open after action succeeds)."* |

## Why this exists

`MergeContactsDialog.handleCommit` (`frontend/src/components/MergeContactsDialog.tsx:103-111`) awaits
`commit(...)` and then calls `onMerged(selectedContact.ID)`. It never calls its own `handleClose`
(`:96-101`) and never calls `onClose`.

The parent's `onMerged` is `navigate(\`/contacts/${keeperId}\`)`
(`frontend/src/ContactDetailPage.tsx:1386`), and `setMergeDialogOpen(false)` appears **only** in the
`onClose` prop at `:1385`. Because `/contacts/:id` renders the same `ContactDetailPage` element
(`frontend/src/App.tsx:486`), changing the route param does **not** unmount the component — so
`mergeDialogOpen` stays `true`.

The result is a dialog sitting over the keeper's page, still holding the now-deleted loser in
`selectedContact` and a stale `preview`, while `currentContactId` has silently become the keeper. Clicking
merge again submits a request built from that stale state.

## What to build

1. In `handleCommit` (`MergeContactsDialog.tsx:103-111`), on success reset `selectedContact` and `preview`
   and close the dialog **before** calling `onMerged`. Reuse `handleClose` (`:96-101`) rather than
   duplicating the reset.
2. In `ContactDetailPage.tsx:1386`, have `onMerged` call `setMergeDialogOpen(false)` explicitly as well as
   navigating. Belt and braces: the dialog's own close is the fix, but the parent owning its own `open`
   state is what stops this recurring the next time someone adds a second entry point
   ([T92](136-T92-bulk-merge-from-contacts-list.md) is adding one).

Do not close the dialog on *failure* — a failed merge should leave the user's choices intact so they can
retry or fix the conflict.

## Traps

- **The same non-unmount is why [T95](139-T95-merge-keeper-shows-stale-circles.md) happens.** Fix them in
  one commit; closing the dialog without also refreshing the keeper's state just moves the symptom.
- New component test files need an explicit `afterEach(cleanup)` (`/CLAUDE.md` frontend trap #1).

## Done when

- Merging from a contact's detail page closes the dialog and lands on the keeper's page.
- A failed merge leaves the dialog open with the selection intact.
- Reopening the dialog after a successful merge shows an empty picker, not the previous selection.
- `cd frontend && npx tsc --noEmit && npx vitest run` green, plus a Playwright spec asserting the dialog is
  gone after a successful merge.
