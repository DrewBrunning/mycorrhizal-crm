# T53 — Delete a contact from its own detail page, not only from the list

| | |
|---|---|
| **Rating** | 3 — real gap; ambiguous same-name contacts make list-only delete a genuine risk, not just inconvenience |
| **Size** | S |
| **Depends on** | — |
| **Alpha** | n/a — real data exists; this is an existing, already-soft-delete backend endpoint, no schema change |
| **Source** | v0.3.0 post-release testing, 2026-08-06 |

## Why this exists

Delete is reachable today, but not where a user would look for it. `ContactHeader.tsx` already has
a delete button (`onDeleteContact`, `DeleteIcon` at `ContactHeader.tsx:274-281`) — but it's placed
inside the inline profile-*edit* sub-panel, next to Save/Cancel, not in the main "…" actions menu a
few lines away (`ContactHeader.tsx:342-400`, which has Stay in Touch / Merge / Prep View / Share /
Export ×3 / Archive — no Delete). Reaching it today means opening profile edit mode first, which
most users have no reason to do just to delete a contact. Functionally this is closer to "delete is
undiscoverable" than "delete doesn't exist," but the effect the report describes — only being able
to delete from the list, where two same-named contacts are genuinely ambiguous — is real either way:
nothing on the list row itself disambiguates two "John Smith"s, so confidently deleting the *right*
one requires seeing the full detail first.

## What to build

- Move (or add) a Delete entry to the main actions `Menu` in `ContactHeader.tsx` (alongside
  Archive/Unarchive), so it's reachable without opening the profile-edit panel.
- Confirm before deleting — this repo's own convention for bulk delete
  (`ContactsPage.tsx:157-162`, `window.confirm` gated) is the precedent to match, not invent a new
  pattern.
- After a successful delete from the detail page, navigate back to the contacts list (per the
  report — the current list-based delete already has nowhere to navigate *from*, so this is new
  behavior specific to the detail-page entry point).
- Decide whether to keep the buried profile-edit-panel delete button too (redundant once the main
  menu has it) or remove it — leaving both is more surface area to keep in sync for no benefit.

## Traps

- This calls the same soft-delete `DELETE /contacts/:id` the list already uses — don't build a
  second delete code path. The only thing missing is a UI entry point, not backend work.
- If the profile-edit-panel button is kept rather than removed, make sure both trigger the exact
  same confirmation/handler rather than diverging into two slightly different delete flows.

## Done when

- `npx tsc --noEmit` clean, `npx vitest run` green.
- e2e coverage: delete a contact from its own detail page, confirm the confirmation prompt appears,
  confirm navigation back to the list afterward, confirm the contact is gone (soft-deleted) from the
  list.
- Hand-verified: create two contacts with the same name, delete one from its detail page, confirm
  the correct one is gone and the other is untouched.
