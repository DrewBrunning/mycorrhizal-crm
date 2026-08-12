# T84 — Custom field values are entirely absent on Android

| | |
|---|---|
| **Platform** | Android |
| **Rating** | 3 — a whole shipped feature (T6/T7) invisible on mobile, but only for users who defined custom fields |
| **Size** | S–M — three client methods plus a value editor per field type |
| **Depends on** | Nothing. Adjacent to [M7](88-M7-android-contact-record-coverage.md) but deliberately separate — see below. |
| **Status** | TO BE DONE |
| **Source** | Found in the 2026-08-12 readiness pass, checking whether [M7](88-M7-android-contact-record-coverage.md)'s field groups needed the custom-field endpoints. They don't — and that exposed a gap nothing else covers. |

## Why this exists

[M8](89-M8-web-android-parity-audit.md)'s audit excluded **custom field *definitions*** — the schema
authoring surface in Data Settings — as deliberately not-on-mobile. Reasonable: defining a field
type is a desk activity.

But it never addressed custom field **values**, which are a different surface entirely. On web they
are part of the contact record itself: `ContactDetailPage.tsx` loads `fieldDefinitions` and
`fieldValuesByDefinition` and renders them through `ContactInformation` with an `onSaveFieldValue`
handler, right alongside the built-in fields.

On Android they do not exist at any layer. `ApiClient`'s 83 methods include **none** of
`GET /field-definitions`, `GET /contacts/:id/field-values`, or `PUT /contacts/:id/field-values` — so
a user who defined "Coffee order" on web sees no trace of it on their phone, and a contact edited on
Android is edited without it.

Per M8's own sign-off rule — parity is the default, exclusions are decided rather than inferred —
this is a gap that should have become a ticket. It is being filed now rather than assumed excluded.
**If it should in fact be excluded, decide that explicitly and close this**; what must not happen is
it staying invisible because it fell between M7's scope and M8's exclusion list.

## Why it's separate from M7

M7 covers the `Card`/`CRM` field groups, which all ride the existing `updateContact` call and need no
new endpoints. Custom fields need three new client methods and a per-field-type value editor driven
by `FieldDefinition.FieldType`. Folding them in would confuse a ticket that is otherwise
endpoint-free, and M7 is already large.

## What to build

| Need | Route | In `ApiClient`? |
|---|---|---|
| List definitions | `GET /field-definitions` | **No** |
| Read a contact's values | `GET /contacts/:id/field-values` | **No** |
| Write them | `PUT /contacts/:id/field-values` | **No** |

1. Add the three client methods.
2. Render each definition's value on the contact detail screen and make it editable, dispatching on
   `FieldDefinition.FieldType` (`models/field_definition.go` — `number` and friends; read the real
   registry rather than assuming a set).
3. **Read-only is an acceptable first slice** if the editor-per-type work is large: showing a value
   the user defined on web is most of the benefit, and it is honest as long as the screen doesn't
   imply editability. Decide which slice is being built and say so in the landing note.

## Traps

- **`FieldDefinition`'s only target is `contact`** (`FieldDefinitionTargetContact`,
  `models/field_definition.go:16`) and its `EntityID` is a `Contact.VCardUID`, never a bare
  `Contact.ID` — the graph invariant every join entity has followed since WP-80. Sending a numeric id
  will not match anything.
- **Field-type lists are hardcoded mirrors of the backend registry** — there is no dynamic type-list
  endpoint anywhere in this codebase, by design (`/CLAUDE.md` frontend trap #4). Add the "must stay
  in sync" comment.
- **Definition *authoring* stays excluded.** This ticket surfaces values for definitions that already
  exist; it does not add a schema editor to the phone.

## Test cases

1. **Round-trip** — MockWebServer for all three methods; a value set on Android is returned by a
   subsequent read.
2. **No definitions** — a user with zero custom fields sees no empty section and no crash, with the
   collection key **absent** from the JSON as well as `[]` (`/CLAUDE.md` frontend trap #8).
3. **Value for a definition that no longer exists** degrades gracefully rather than crashing — the
   definition list and the value list are fetched separately and can disagree.
4. **Per-type rendering** — at least two different `FieldType`s render with their appropriate input.

### Gate

- `./gradlew testDebugUnitTest`, `./gradlew lintDebug`, `./gradlew assembleDebug` — the exact three
  steps `.github/workflows/android-tests.yml` runs.
- Every new user-facing string in all five locales (`values`, `values-de/es/fr/it`).

### Test conventions (this repo, not generic)

JUnit4 + MockK (`mockk`/`coEvery`) + Turbine + `runTest` with `MainDispatcherRule`. ViewModel tests
mock the repository — `feature/contacts/.../ContactListViewModelTest.kt` is the reference. New
`ApiClient` methods get a MockWebServer test in `core/network` — `ApiClientTest.kt` is the reference.
Hand-verify per `/CLAUDE.md`: break the code, confirm the new test fails, restore.
