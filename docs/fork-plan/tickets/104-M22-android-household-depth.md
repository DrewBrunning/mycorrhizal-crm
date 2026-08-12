# M22 — Household management depth on Android

| | |
|---|---|
| **Rating** | 3 |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | Nothing |
| **Status** | TO BE DONE |

Household core CRUD (create/edit/delete household, add/remove member) already has native parity.
What's missing, per `HouseholdsPage.tsx`/`HouseholdList.tsx`/`AddressHouseholdSuggestions.tsx`:

## Scope

- **Change an existing member's role in place** (`HouseholdsPage.tsx:110-116`,
  `HouseholdList.tsx:248-264`) — `HouseholdDetailViewModel` has `addMember`/`removeMember` only, no
  update-role method; today the only way to change a role on Android is remove-then-re-add.
- **Resolve/display member names**, not raw vCard UIDs — `HouseholdDetailScreen.kt:150-156`
  currently shows `memberVCardUid` as a raw string. Same category of gap as
  [M21](103-M21-android-relationships-depth.md)'s relationships-list finding; consider sharing the
  name-resolution approach between the two tickets if one lands first.
- **Add member: contact search/autocomplete**, not a raw free-text UID field
  (`HouseholdList.tsx:45-116` vs. `HouseholdDetailScreen.kt:176-217`). Same for role — a
  constrained/translated picker from `HOUSEHOLD_ROLES`, not free text.
- **AI/heuristic relationship suggestions within a household** (`HouseholdsPage.tsx:93-107`,
  `HouseholdList.tsx:198-215`) — no equivalent anywhere in Android's households module.
- **T40 shared-address household suggestions**: scan, accept (materialize household), dismiss
  (`HouseholdsPage.tsx:118-165`, `AddressHouseholdSuggestions.tsx`) — no equivalent anywhere on
  Android.

## Done when

- Role can be changed on an existing member without remove-and-re-add.
- Member names resolve and are legible (not raw UIDs) in the household detail screen.
- Adding a member uses contact search, not manual UID entry.
- T40 address-based suggestions are reviewable (accept/dismiss) from Android.
- Hand-verified on-device against an instance with an existing household and at least one pending
  T40 suggestion.
- New strings translated in all five locales.

---

## Implementation contract (added 2026-08-12)

Added in the readiness pass: this ticket came out of the [M8](89-M8-web-android-parity-audit.md)
parity audit, whose job was to prove a gap was real — not to specify the build. Endpoints, test
cases and the CI gate below close that difference.

### Endpoints

| Need | Route | In `ApiClient`? |
|---|---|---|
| Suggest relationships in a household | `POST /households/:id/suggest-relationships` | **No** |
| Suggest households from shared address | `POST /households/suggest-addresses` | **No** |
| Accept a suggestion | `POST /households/suggestions/accept` | **No** |
| Dismiss a suggestion | `POST /households/suggestions/dismiss` | **No** |
| Member role editing | `PATCH /households/:id/members/:vcard_uid` | Yes (`updateHouseholdMember`) |
| Household CRUD + membership | — | Yes |

Four new client methods; role editing is already wired at the client layer and just needs UI.

### Read T64 before building the suggestion screens

[T64](90-T64-household-suggestions-null-crash.md): "Suggest Households" crashed the **entire web app**
when there was nothing to suggest — a nil slice serialized as an absent key, and no client guarded it.
Both suggestion endpoints here are the same shape. Guard the empty case from the start and test it
against raw JSON with the key omitted.

### Test cases

1. **Empty suggestions render an empty state** — with the collection key **absent** from the JSON, not
   merely `[]`. This is T64 verbatim; decoding into a Kotlin object with a default makes the two
   indistinguishable, which is exactly how the web test passed while the bug shipped.
2. **Accept** removes the suggestion and the household reflects the new member.
3. **Dismiss** removes it and it does not return on refresh.
4. **Role editing** round-trips through `updateHouseholdMember`.
5. **Name resolution** — members render as names, not UIDs (same gap as
   [M21](103-M21-android-relationships-depth.md); reuse whatever it builds).

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
