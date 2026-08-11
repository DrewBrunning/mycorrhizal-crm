# M21 — Relationships depth on Android

| | |
|---|---|
| **Rating** | 4 |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | Nothing |
| **Status** | TO BE DONE |

`RelationshipsScreen` covers type-select, accept-suggested, and create/delete natively. What's
missing, per `RelationshipEdgeDialog.tsx`/`RelationshipEdgeList.tsx`:

## Scope

- **Search-based linking to an existing contact.** Web offers an autocomplete search
  (`RelationshipEdgeDialog.tsx:259-290`); Android requires manually typing/pasting the other
  party's raw vCard UID (`RelationshipsScreen.kt:240-245`) — effectively undiscoverable without
  already knowing the UID.
- Gender and birthday fields on manual (non-linked) entry creation
  (`RelationshipEdgeDialog.tsx:234-243,292-347`).
- Sensitivity select (normal/private/secret, `:349-360`) — no field on Android's create/edit path.
- **Edit an existing relationship** (type, sensitivity) — `RelationshipsViewModel.kt` has no update
  method at all today; create, accept, and delete are the only operations.
- **Other-party name resolution.** `RelationshipsScreen.kt:158-162` renders the raw vCard UID
  string, not the resolved contact name, and it isn't tappable/navigable — web resolves and
  links (`RelationshipEdgeList.tsx:56-63,92-105`). This is probably the highest-value single item
  here: a relationships list full of raw UIDs is close to unusable.
- Distinct reject action for a suggested edge, separate from delete (currently both use the same
  generic delete button/copy — `RelationshipEdgeRepository.kt:25` documents that delete "also
  rejects a suggestion" under the hood, so the backend call is fine; only the UI/copy distinction is
  missing).
- Confirmed/suggested section split on the list (currently a flat list with an inline "suggested"
  label + accept chip, not sectioned like web).
- Delete-confirmation dialog (currently immediate, no confirm, unlike every other delete on web).

## Done when

- Relationship names resolve and navigate correctly — this alone should ship first if the ticket
  gets split, since everything else compounds on top of an unreadable list.
- Linking to an existing contact works via search, not manual UID entry.
- Edit works for type and sensitivity on an existing confirmed edge.
- Hand-verified on-device: link via search, edit sensitivity, reject a suggestion (confirm it's
  distinguishable from deleting a confirmed edge), delete with confirmation.
- New strings translated in all five locales.
