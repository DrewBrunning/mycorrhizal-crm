# M17 — Android entity-list scaffold: add edit + delete-confirmation

| | |
|---|---|
| **Rating** | 4 — one fix resolves four entity types at once |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 — structural finding #3 |
| **Depends on** | Nothing |
| **Status** | TO BE DONE |

Life Events, Gifts, Preferences, and Conversation Agenda all share one generic Android scaffold —
`EntityListScreens.kt` (`android/feature/timelineentities/`) backed by one generic
`EntityListUiState`/`EntityItem` shape in `TimelineEntitiesViewModel.kt:39-48`. That shared scaffold
structurally has **no edit affordance** and **no delete-confirmation dialog**: `EntityListScreens.kt`
wires create (FAB → dialog) and delete (immediate, no confirm, `:140-142`) only. Every one of the
four entities inherits both gaps identically. Fixing the scaffold once is far cheaper than four
separate per-entity tickets for the same two problems — this ticket is scoped to exactly that,
before [M18](100-M18-android-entity-field-richness.md) adds the per-entity field depth each of
these four still lacks even after this lands.

## Scope

1. Add a delete-confirmation `AlertDialog` to the shared scaffold (web confirms via
   `window.confirm` on all four — `LifeEventList.tsx:90-92`, `GiftList.tsx:208-212`,
   `PreferenceList.tsx:74-81`, `ConversationAgendaList.tsx:68-72,90-97`). One implementation, reused
   by all four call sites.
2. Add a tap-to-edit path to the shared scaffold (currently rows have no click handler at all in
   three of the four; only gifts render an "open URL" icon action, which is unrelated). This can
   reuse each entity's existing create-dialog component in edit mode, the same pattern web's
   `EditTimelineItemDialog.tsx` uses relative to its add-dialogs.

## Done when

- Deleting any of the four entity types on Android requires confirmation, matching web.
- Editing any of the four entity types is reachable by tapping the row, at minimum for the fields
  the current create dialog already models (field-level richness is M18's job, not this ticket's).
- A single shared-scaffold change accounts for the fix landing on all four entities — not four
  separate patches.
- Hand-verified on-device for each of the four entity types: create, edit, attempt delete-and-cancel,
  delete-and-confirm.

---

## Implementation contract (added 2026-08-12)

Added in the readiness pass: this ticket came out of the [M8](89-M8-web-android-parity-audit.md)
parity audit, whose job was to prove a gap was real — not to specify the build. Endpoints, test
cases and the CI gate below close that difference.

### Endpoints

**None.** Every method this ticket needs is already in `ApiClient`: `updateGift`/`deleteGift`,
`updateLifeEvent`/`deleteLifeEvent`, `updatePreference`/`deletePreference`,
`updateConversationAgenda`/`deleteConversationAgenda`. This is purely a UI-layer gap in the shared
scaffold — which is what makes it cheap and what makes it fix four entity types at once.

### Surface

The shared entity-list scaffold in `feature/timelineentities/` (`EntityListScreens.kt`,
`TimelineEntitiesViewModel.kt`). Fix the scaffold, not the four call sites.

### Test cases

1. **Delete asks first** — tapping delete shows a confirmation and does **not** call the repository
   until it is confirmed. This is the whole point; a test that only asserts "delete calls the repo"
   would pass against the current unsafe behavior.
2. **Cancel is inert** — dismissing the dialog leaves the item present and issues no call.
3. **Edit round-trips** — opening edit pre-fills from the loaded entity and saving issues the update
   with the edited values.
4. **Parameterize 1–3 across all four entity types.** The claim being tested is that *one* scaffold
   fix resolves all four; per-entity tests for a single type would not establish it. This also
   unblocks [M18](100-M18-android-entity-field-richness.md), which assumes edit exists.

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
