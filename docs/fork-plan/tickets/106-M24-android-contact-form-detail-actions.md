# M24 — Contact form & detail-page actions on Android

| | |
|---|---|
| **Platform** | Android |
| **Rating** | 4 — delete/archive missing from the detail page is a real gap, not polish |
| **Size** | S — 3 new endpoints, each a straightforward action plus a confirm |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | Nothing |
| **Status** | **IMPLEMENTED, AWAITING ON-DEVICE VERIFICATION** (2026-08-13). All three new `ApiClient` methods, the repository layer, the detail-screen action menu with confirmations, inline circle/tag editors, and the form fields landed; `testDebugUnitTest`/`lintDebug`/`assembleDebug` green. The ticket's on-device hand-verify step is still outstanding — no device/emulator available in the build environment. |

This is distinct from [M7](88-M7-android-contact-record-coverage.md) — M7 is the depth gap *inside*
the `Card`/`CRMEnvelope` record (addresses, orgs, links, etc.); this ticket is the set of top-level
actions and form fields around the record that M7 explicitly excluded.

## Scope

**Contact form** (`ContactFormViewModel.ContactFormState` vs. `AddContactDialog.tsx`):
- Prefix / middle name / suffix fields — not in `ContactFormState` at all.
- Kind (human/animal) — new Android contacts always default to the backend's default kind.
- Language field.
- Circles: autocomplete of existing circles, not a free-text comma-separated field
  (`ContactFormScreen.kt:195-201`).
- Tags field — entirely absent from the Android create/edit form (only reachable in reverse, from a
  tag's own detail screen).

**Contact detail page top-level actions** — none of these exist at the `ContactRepository`
interface level today, not just missing UI:
- **Delete contact**, with confirmation.
- **Archive / unarchive contact**, with confirmation.
- Export contact (vCard 4.0 / vCard 3.0 / JSContact download).
- "Stay in touch" one-tap quick action (pre-fills a recurring reminder — distinct from manually
  creating one via the Reminders screen).
- Profile-picture upload (with crop; web additionally offers Immich linking — Immich itself was
  marked deliberately-not-on-mobile at M8 sign-off, so a plain upload/crop path is sufficient here,
  no Immich picker needed).
- Inline circle chip editor on the detail page (add/remove without going through the full edit
  form).
- Inline tag chip editor on the detail page (currently tags aren't even displayed read-only on
  Android's contact detail screen).
- Share-contact entry point (wires into [M15](97-M15-android-contact-sharing.md) once that lands —
  this ticket can add the menu item as a stub pointing at it, or land after M15, whichever is more
  convenient at implementation time).
- View-prep entry point (wires into [M11](93-M11-android-prep-view.md) similarly).

## Done when

- Delete and archive/unarchive both work from the contact detail screen, with confirmation,
  matching web's semantics (soft-delete per `/CLAUDE.md`'s delete-semantics rules — this is calling
  the existing backend endpoint, not inventing new delete behavior).
- Export produces a file matching one of web's three formats.
- Circles and tags are both visible and editable inline from the detail page, not only via the full
  edit form.
- Hand-verified on-device: archive a contact, confirm it's excluded from the default list; delete a
  contact, confirm the standard soft-delete/undo story holds (check against T60's audit undo once
  M16 lands, or against the web audit page in the meantime).
- New strings translated in all five locales.

---

## Implementation contract (added 2026-08-12)

Added in the readiness pass: this ticket came out of the [M8](89-M8-web-android-parity-audit.md)
parity audit, whose job was to prove a gap was real — not to specify the build. Endpoints, test
cases and the CI gate below close that difference.

### Endpoints

| Need | Route | In `ApiClient`? |
|---|---|---|
| Delete a contact | `DELETE /contacts/:id` | **No** |
| Archive | `POST /contacts/:id/archive` | **No** |
| Unarchive | `POST /contacts/:id/unarchive` | **No** |

**All three absent from the client** — confirming the ticket's claim that this is a repository-level
gap, not missing UI. Verified by diffing `ApiClient`'s 83 methods against the route table.

### Delete semantics

`Contact` **soft-deletes** (`/CLAUDE.md`: content the user authored soft-deletes; the row is someone's
undo). Do not present it as permanent, and do not expect the row to vanish from a backend count —
`Unscoped()` is what distinguishes gone from marked.

### Test cases

1. **Delete round-trip** — MockWebServer; the contact leaves the list, and on failure it stays and an
   error surfaces.
2. **Delete is confirmed first**, and the confirmation names the contact. Deleting the wrong contact
   from a list is the failure mode.
3. **Archive/unarchive** round-trip and flip the contact's presence in the default (non-archived)
   list.
4. **Post-delete navigation** — deleting from the detail screen navigates back rather than leaving a
   screen bound to a deleted contact.

### Gate

- `./gradlew testDebugUnitTest`, `./gradlew lintDebug`, `./gradlew assembleDebug` — the exact three
  steps `.github/workflows/android-tests.yml` runs. CI has been green since M1's review pass; keep it.
- Every new user-facing string in all five locales (`values`, `values-de/es/fr/it`). M1's review pass
  had to retrofit ~80 unlocalized strings — don't rebuild that debt.

### Test conventions (this repo, not generic)

JUnit4 + MockK (`mockk`/`coEvery`) + Turbine + `runTest` with `MainDispatcherRule`. ViewModel tests
mock the repository — `feature/contacts/.../ContactListViewModelTest.kt` is the reference. New
`ApiClient` methods get a MockWebServer test in `core/network` — `ApiClientTest.kt` is the
reference. Hand-verify per `/CLAUDE.md`: break the code, confirm the new test fails, restore.

---

## Landing note (2026-08-13)

**What landed** (all in `android/`; no backend changes were needed — every endpoint pre-dates
the client):

- **`ApiClient`**: `deleteContact`, `archiveContact`, `unarchiveContact`, and
  `exportContactVcf(vcardUid, version)` (raw bytes via a new `executeGetBytes` helper; vCard
  4.0 default, 3.0 when `version=3`), each with MockWebServer tests.
- **`ContactRepository`** (+ impl): the four methods above, online-first with cache sync —
  delete removes the cached row, archive/unarchive flip the cached `archived` flag (`setArchived`).
- **`CircleRepository`/`TagRepository`**: `circlesForContact(uid)` and `tagsForContact(uid)`
  derive a contact's circles/tags from the `include_members`/`include_contacts` snapshots,
  because the contact payload carries neither (its `crm.circles` is the stale flat mirror, and
  tags aren't in the detail response at all). This is the *authoritative* source the detail
  editors and the edit form both seed from.
- **Contact detail screen**: a ⋮ action menu (archive/unarchive, export vCard 4.0/3.0, stay in
  touch, share + prep stubs, delete). Delete and archive/unarchive are confirmed first and name
  the contact; delete navigates back on success. Export writes the file to the cache and hands
  it to the share sheet via a new `FileProvider` (`${applicationId}.fileprovider` +
  `res/xml/file_paths.xml`). Stay-in-touch navigates to the reminder form pre-filled with
  "Catch-up with \<name\>" + quarterly recurrence (`ReminderFormViewModel` reads optional
  `message`/`recurrence` SavedStateHandle args; new route query params).
- **Inline circle/tag editors** on the detail page: removable chips + an add dropdown of the
  not-yet-applied entities, writing through the repositories.
- **Contact form**: prefix/middle name/suffix fields (map to name components `title`/`given2`/
  `generation`, matching web's AddContactDialog), kind (human/animal → `crm.kind`, default
  human), language (→ `card.language`, defaulted to the device locale on create), and
  circles/tags as *autocomplete-of-existing* chips + dropdown (the ticket's explicit
  requirement) replacing the free-text comma-separated circles field. On save, memberships are
  reconciled through the join-row sub-resources (add newly selected, remove deselected) — the
  PUT's `crm.circles` only touches the legacy flat column. The T81 "copy onto the loaded entry,
  never reconstruct" rule is preserved for the new name components too (only the editable kinds
  are replaced).
- **i18n**: 32 new keys in all five locales (`values`/`-de/-es/-fr/-it`), `LocalesConsistencyTest`
  green; `contact_circles_label` removed everywhere (the comma-separated field it labeled is gone).

**Test coverage** (all hand-verified per `/CLAUDE.md` — each listed direction was broken and
confirmed to fail before restore): ApiClient MockWebServer round-trips incl. error mapping and
the `version=3` param; repository cache-sync tests (delete removes row, failure keeps it;
archive/unarchive flip the flag); the join-row derivations (blank-uid short-circuit, snapshot
filtering, failure propagation); detail-VM action tests (delete → `ContactDeleted`, archive →
reload, export → `ExportReady`, error paths, no-uid guard, add/remove → repo + re-derivation);
form-VM tests (component mapping, kind/language payload, create-membership adds, edit-mode
deselect removes, plus all pre-existing T81 regressions kept green); and screen tests for the
chips/add-dropdown. CI gate (`testDebugUnitTest` + `lintDebug` + `assembleDebug`) green with
`--rerun-tasks`.

**Deliberate deviations / deferrals** (nothing here is in the ticket's "Done when", so none
blocks this landing, but each is recorded per the readiness pass's "state it rather than
discover it" rule):

- **Profile-picture upload deferred.** It's in the Scope list ("plain upload/crop path is
  sufficient"), but a crop UI would require either a new dependency (uCrop) or a hand-rolled
  Compose cropper — neither is safe to add blind in an offline build environment, and the
  ticket's "Done when" doesn't include it. The `POST /contacts/:id/profile_picture` endpoint
  is already reachable from `ApiClient`'s existing multipart machinery when it's built.
- **Export offers vCard 4.0 and 3.0 only, not JSContact.** The backend's `/export/jscontact`
  ignores `?vcard_uid=` (unlike `/export/vcf`), so a "single-contact JSContact export" would
  silently produce a file of *all* contacts. Web's `exportContact('jscontact', uid)` has the
  same latent issue; it was deliberately not replicated on mobile. "One of web's three formats"
  is satisfied by vcf4.
- **Share-contact and view-prep are stub menu items** showing the standard "coming in a later
  phase" snackbar, per the ticket's "can add the menu item as a stub" option (M15/M11 are
  separate tickets).
- **Membership derivation uses a single 100-item page** (`limit=100` — the backend's
  `GetCursorParams` clamps to `maxLimit=100` regardless of the request, and 100 is web's own
  `listCircles`/`listTags` default). Not cursor-walked; a user with >100 circles/tags would see
  truncation (documented in the repository doc comments).
- **On-device hand-verification is outstanding**, matching M21's status: no device/emulator in
  the build environment. The ticket's on-device steps (archive → excluded from the default
  list; delete → soft-delete/undo story; name-linking in the delete confirm) are exactly what
  the unit/UI tests pin, but a real-device pass is still owed.

## On-device verification (2026-08-14, Pixel 8a)

Verified against the real account, using a throwaway contact created for this purpose
("ZZTEST DeleteMeVerify") rather than any real contact — real contacts were only used for
read-only checks (opening the ⋮ menu, viewing the export share sheet) without confirming any
destructive action. The ⋮ menu shows all seven items (Archive, Export vCard 4.0, Export vCard
3.0, Stay in touch, Share contact, View prep, Delete contact). "Export vCard 4.0" on a real
contact produced a working share sheet with a correctly named `.vcf` file attached. On the
throwaway contact, "Archive" showed a confirmation naming the contact exactly
("Are you sure you want to archive ZZTEST DeleteMeVerify? …") and warning about reminder loss,
matching the ticket's semantics; confirming archived it successfully. The contact form's
prefix/given/surname/middle/suffix/nickname/kind/language fields and the circles/tags
autocomplete-of-existing sections (replacing the old free-text field) are all present and were
used to create the throwaway contact. Delete was not exercised on this pass (the device locked
mid-session); the archived throwaway contact needs a manual delete as cleanup.
