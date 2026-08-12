# M24 — Contact form & detail-page actions on Android

| | |
|---|---|
| **Rating** | 4 — delete/archive missing from the detail page is a real gap, not polish |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | Nothing |
| **Status** | TO BE DONE |

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
`ApiClient` methods get a MockWebServer test in `core/network` — `ApiClientTest.kt` is the reference.
Hand-verify per `/CLAUDE.md`: break the code, confirm the new test fails, restore.
