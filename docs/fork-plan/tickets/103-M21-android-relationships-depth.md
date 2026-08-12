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

---

## Implementation contract (added 2026-08-12)

Added in the readiness pass: this ticket came out of the [M8](89-M8-web-android-parity-audit.md)
parity audit, whose job was to prove a gap was real — not to specify the build. Endpoints, test
cases and the CI gate below close that difference.

### Endpoints

| Need | Route | In `ApiClient`? |
|---|---|---|
| Update an edge | `PUT /relationship-edges/:id` | **No** |
| Resolve names in bulk | `GET /contacts?vcard_uid=…` (repeatable) | Check `listContacts` |
| List / create / delete / accept | `GET,POST,DELETE /relationship-edges`, `PATCH /:id/accept` | Yes |

**The raw-UID display bug is a name-resolution gap, and the endpoint for it already exists.**
`GET /contacts?vcard_uid=` is a repeatable batch by-UID lookup built for exactly this — web's
relationship list uses it to turn `Contact.VCardUID` into a display name.

**Resolved 2026-08-12**: `ApiClient.listContacts` (`ApiClient.kt:125-136`) accepts only `cursor`,
`limit`, `search` and `includeArchived` — **`vcard_uid` is not exposed**. Adding it is the fix, and
it must be *repeatable* (one `vcard_uid` query parameter per UID), which `HttpUrl.Builder`
`addQueryParameter` supports by calling it once per value. Note the handler short-circuits its whole
search/sort/pagination path for this parameter, so don't send `cursor`/`limit` alongside it.

### Domain rules — get these right or the labels are backwards

- `RelationshipEdge.Type` describes the **source's** role relative to the target. `parent_of` from A
  to B means "A is B's parent."
- Only one direction is stored; the inverse is derived from
  `models/relationship_type_registry.go` and never persisted.
- Creating from a contact's page sends `target_id: <viewed contact>`, so a dropdown label always
  describes the **other** party.
- Only `status: confirmed` edges are fact. `suggested` edges must never be shown as real outside a
  review surface.

### Test cases

1. **Names, not UIDs** — an edge whose other party is a known contact renders that contact's name and
   is tappable through to them. Assert the rendered text is not the UID; this is the highest-value
   item in the ticket.
2. **Unresolvable UID** degrades to something readable rather than a raw UID or a crash.
3. **Direction** — an edge created from contact A's page toward B renders with the label describing
   **B's** role from A's view, and the inverse from B's page. A symmetric type (`sibling_of`) cannot
   detect a direction bug — use an asymmetric one (`parent_of`).
4. **Suggested edges** are visually distinct and are not counted among confirmed relationships.
5. **Edit** — `PUT` round-trips a changed type and preserves the edge's other fields.

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
