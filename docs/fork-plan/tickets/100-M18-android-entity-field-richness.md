# M18 — Field richness: Life Events, Gifts, Preferences, Conversation Agenda on Android

| | |
|---|---|
| **Rating** | 3 — large; consider splitting per entity once scoped for real |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | [M17](99-M17-android-entity-scaffold-edit-delete-confirm.md) (edit needs to exist before these fields are worth adding to an edit form, not just create) |
| **Status** | TO BE DONE |

Each of the four entities sharing Android's `EntityListScreens.kt` scaffold has a create dialog
that models only a fraction of its web counterpart's fields. This ticket is the field-by-field
depth work; it's deliberately kept as one ticket for now since all four are small and share a
pattern, but split it into four if it turns out too large to land together — that's an
implementation-time call, not a design one.

## Scope, per entity

**Life Events** (`LifeEventDialog.tsx` vs. `EntityListScreens.kt:163-209`, currently type+
description only):
- Category select (5 fixed categories + legacy "uncategorized", `LifeEventDialog.tsx:267-288`).
- Type select scoped per category with a custom-type escape hatch (`:310-337`) — currently free text
  only.
- Partial date (year/month/day, any subset present, `:340-365`) — currently no date field at all.
- Related contacts, multi-select (`:376-415`).
- "Remind me" checkbox, enabled only once month+day are set (`:417-436`).

**Gifts** (`GiftDialog.tsx` vs. `EntityListScreens.kt:211-252`, currently description only):
- Status/section (idea/purchased/given/received) — currently every gift lands wherever `GiftInput`
  defaults to.
- URL, notes, occasion, date.
- Amount + currency.
- Life-event and activity links.
- Mark-given one-click status transition (`GiftList.tsx:232-244`).
- Clothing sizes panel (`ClothingSizesPanel.tsx`) — currently no surface at all on Android, not even
  folded into the generic Preferences screen.

**Preferences** (`PreferenceDialog.tsx` vs. `EntityListScreens.kt:254-302`, currently free-text
category+value only):
- Category select from the fixed token list (currently free text).
- Key autocomplete, freeSolo, category-scoped defaults (`:114-128`) — currently no key field at all.
- Sensitivity select (normal/private/secret, `:129-139`) — currently every Android-created
  preference gets whatever the backend defaults to.
- Section grouping on the list (Food & Drink / Media / Other, `PreferenceList.tsx:13-33,101-106`).

**Conversation Agenda** (`ConversationAgendaDialog.tsx` vs. `EntityListScreens.kt:304-345`,
currently content-only):
- Reference URL on create/edit.
- Mark-discussed action, linking to an activity (`ConversationAgendaList.tsx:74-86`,
  `MarkDiscussedDialog`) — currently no "discussed" state exists on Android at all.
- Open/discussed section split on the list.

## Done when

- Each field above is creatable/editable on Android and round-trips correctly with web.
- Sensitivity gating on preferences is respected the same way it is everywhere else per
  `/CLAUDE.md`'s security posture note (excluded from exports/sync above `normal`, in the query).
- Hand-verified on-device per entity: create with every new field populated, confirm it renders
  correctly on web.
- New strings translated in all five locales.

---

## Implementation contract (added 2026-08-12)

Added in the readiness pass: this ticket came out of the [M8](89-M8-web-android-parity-audit.md)
parity audit, whose job was to prove a gap was real — not to specify the build. Endpoints, test
cases and the CI gate below close that difference.

### Endpoints

**None new for the four entities themselves** — `updateGift`, `updateLifeEvent`, `updatePreference`
and `updateConversationAgenda` are already in `ApiClient`. This ticket adds *fields to the forms*,
not calls.

**Resolved 2026-08-12**: custom fields do not apply here. `FieldDefinition`'s only target is
`FieldDefinitionTargetContact = "contact"` (`models/field_definition.go:16`) and its `EntityID` is a
`Contact.VCardUID` — so Life Events, Gifts, Preferences and Agenda items cannot carry custom fields
at all. Every field this ticket adds is a built-in on those models and rides the existing update
calls. **This ticket needs no new endpoints.**

### Blocked on M17

Edit has to exist before fields are worth adding to an edit form. Don't start this first.

### Test cases

1. **Each added field round-trips** — set it, save, reload, assert it survives. Parameterize over the
   fields rather than writing one test per field.
2. **Unmodeled fields survive a save** — load an entity carrying a field the form does not show, edit
   a different field, save, and assert the unshown one is intact. This is
   [T81](125-T81-android-contact-edit-corrupts-phone-email-metadata.md)'s bug in a different form:
   these editors must `.copy()` the loaded object, never reconstruct it.
3. **Empty vs absent** — clearing an optional field persists as cleared rather than being dropped.

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
