# M9 — Wire up already-built Android screens & dead code

| | |
|---|---|
| **Rating** | 4 — cheap, high value; every item here is already built and tested, just unreachable |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 — "wire it up" bucket |
| **Depends on** | Nothing |
| **Status** | TO BE DONE |

M8's audit found four places where the implementation already exists — model, ViewModel,
backend call — and simply isn't wired to a UI entry point. No design work, no new backend calls,
just connecting what's there.

## Scope

1. **Global Notes/Activities drawer routes.** `MycorrhizalApp.kt:454-455` wraps the `"notes"` and
   `"activities"` composables in `PlaceholderScreen`. The real, full `NotesScreen`/`ActivitiesScreen`
   implementations exist and work — they're just wired only under
   `contacts/{contactId}/notes`/`activities`. Point the drawer routes at a
   contact-agnostic entry (an "unfiled + all contacts" view matching web's `NotesPage.tsx`/
   `ActivitiesPage.tsx` inboxes, or at minimum a contact picker into the existing screens) rather
   than a stub.
2. **Bulk circle/tag actions.** `BulkOperationsViewModel.run()` and the `BulkContactOperationInput`/
   `BulkActions` model already support `add_circle`/`remove_circle`/`add_tag`/`remove_tag` — the
   backend call is real. `BulkOperationsScreen`'s UI (`MergeAndBulkScreens.kt:199-212`) only renders
   three buttons: archive/unarchive/delete. Add circle/tag pickers and wire them to the existing
   `run()` calls.
3. **Contact-list pagination past page 1.** `ContactListViewModel.loadNextPage()`
   (`ContactListViewModel.kt:122-153`) is implemented and unit-tested
   (`ContactListViewModelTest.kt:88,97,106-107`) but has no call site anywhere in
   `ContactListScreen.kt`. Wire a scroll-triggered (or button) call so contact lists longer than one
   page are actually reachable — this is a real defect today, not just a missing nicety, on any
   instance with enough contacts to paginate.
4. **VCF upload import.** `ApiClient.uploadVcfImport()` (`ApiClient.kt:601-603`) hits the same
   backend `POST /contacts/import/vcf/upload` endpoint web's VCF import uses, but has zero callers.
   Wire a file-picker entry point (likely alongside the existing device-contacts `ImportContactsScreen`)
   that calls it. Full CSV import + column mapping is **out of scope** here — see
   [M26](109-T65-web-circle-tag-rename-delete.md)'s sibling note; CSV/VCF *file* import was marked
   deliberately not on mobile as part of M8's sign-off. VCF-only is in scope because the client call
   already exists and needs only a picker, unlike CSV which needs a whole mapping UI.

   **Correction while implementing:** re-check the M8 sign-off decision before doing item 4 — CSV/VCF
   file-based import as a *category* was marked deliberately-not-on-mobile. If VCF-only doesn't
   survive that read, drop item 4 and note it in the landing note; the dead `uploadVcfImport()` call
   is a much smaller cleanup either way (remove it, or leave it for a future call site).

## Done when

- All four items reachable from the UI, each pinned by a test that fails if the wiring is removed
  (mirroring the existing unit test for `loadNextPage()`).
- Hand-verified on-device per `/CLAUDE.md`'s workflow section.
- New user-facing strings translated in all five locales.
