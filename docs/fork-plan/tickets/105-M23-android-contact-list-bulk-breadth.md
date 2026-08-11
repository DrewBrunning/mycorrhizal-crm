# M23 — Contact list & bulk breadth on Android

| | |
|---|---|
| **Rating** | 3 |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | [M9](91-M9-android-wire-up-existing-screens.md) item 2 (bulk circle/tag wiring) should land first — this ticket's merge/breadth work is unrelated but touches the same screens |
| **Status** | TO BE DONE |

`ContactListScreen`/`BulkOperationsScreen`/`MergeContactsScreen`/`MergeAndBulkScreens.kt` cover
search, add, archive/unarchive, and delete-with-confirm natively. Remaining gaps, per
`ContactsPage.tsx`/`BulkActionsBar.tsx`/`MergeContactsDialog.tsx`:

## Scope

- **Circle filter dropdown** on the main contact list (`ContactsPage.tsx:172-186`) — absent on
  Android entirely.
- **Archived-contacts toggle** (`ContactsPage.tsx:187-197`) — Android never requests
  `includeArchived`; archived contacts have no visibility control.
- **Per-row select on the main list** — web's bulk-select is inline on the contact list itself
  (`ContactsPage.tsx:273-279,234-250`); Android's only exists as a separate `BulkOperationsScreen`
  with its own unpaginated, unfiltered contact fetch. Consider whether to bring select-mode onto
  the main list (matching web's UX) rather than keeping it a fully separate screen — a design call
  worth making explicit before implementing, not a strict requirement to match web's structure
  exactly.
- **"Select all" in bulk mode** (`ContactsPage.tsx:104-125`) — Android requires tapping every
  contact individually.
- **Merge: search-based target picker** (`MergeContactsDialog.tsx:60-94,129-157`) — Android
  requires typing the target contact's raw numeric ID (`MergeAndBulkScreens.kt:82-89`).
- **Merge: full association-count breakdown** — web shows ~11 categories
  (`MergeContactsDialog.tsx:113-119,238-247`); Android shows only notes/activities/edges
  (`MergeAndBulkScreens.kt:96-107`).

## Done when

- Circle filter and archived toggle both work on the main contact list.
- Merge target is picked by search, not typed ID.
- Merge preview shows the same association categories web does (household, circle, tag, life-event,
  field-value, sync-link counts, not just notes/activities/edges).
- Hand-verified on-device: filter by circle, show archived, merge two contacts using search-based
  target selection.
- New strings translated in all five locales.
