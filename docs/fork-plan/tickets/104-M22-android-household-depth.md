# M22 — Household management depth on Android

| | |
|---|---|
| **Platform** | Android |
| **Rating** | 3 |
| **Size** | M — 4 new endpoints and two suggestion surfaces |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | Nothing |
| **Status** | **DONE** (2026-08-14 — see the landing note below) |

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

---

## Landing note (2026-08-14)

Landed on `feature/M22-android-household-depth`. All five Done-when items shipped; on-device
hand-verification against an instance with a household and a pending T40 suggestion is still
outstanding (same status as M23's landing note — see the board).

**Endpoints.** The four new client methods from the contract landed in `ApiClient` and are threaded
through `HouseholdRepository`/`HouseholdRepositoryImpl` (which also mirrors an accepted household
into the Room cache, matching the module's online-first pattern):

- `suggestHouseholdRelationships(id)` — `POST /households/{id}/suggest-relationships`, empty body.
- `suggestAddressHouseholds()` — `POST /households/suggest-addresses`, empty body.
- `acceptHouseholdSuggestion(input)` — `POST /households/suggestions/accept`, unwraps `{household}`.
- `dismissHouseholdSuggestion(input)` — `POST /households/suggestions/dismiss`.

Role editing (`PATCH /households/{id}/members/{vcard_uid}`) already existed at the client layer
(`updateHouseholdMember`); this ticket gave it its first consumer.

**Name resolution reuses M21's approach wholesale** (`ContactRepository.resolveByUid`), including its
degrade-gracefully policy: unresolved members render as "Unknown contact" rather than the raw UID,
and a resolution failure is not surfaced as the screen's error. Rows resolve to tappable contact
navigation via the same `onNavigateToContact` route the relationships list uses.

**Design decisions taken:**
- The "suggest relationships" trigger lives in the household **detail** screen's top-bar action
  (disabled below 2 members, spinner while running), not on the list card like web — the detail
  screen is where membership is actually visible on Android, so the disable-condition is free.
  The generated edges are `suggested`-status edges reviewed on the members' contact pages, exactly
  as web's message text says.
- Add-member is a debounced contact search + a constrained `ExposedDropdownMenuBox` role picker over
  `HOUSEHOLD_ROLES` (web's `['adult','child','pet','roommate']`, mirrored as `HouseholdRoles`).
  Existing members are excluded from results and also guarded at the confirm button.
- T40 suggestions render as a section above the household list (header + cards with resolved member
  names, the shared address line, Accept/Dismiss). Accept removes the card and refreshes the list so
  the materialized household appears; dismiss removes the card. Both hit the same `(address_hash,
  member_hash)` identity web uses.
- **T64 was honored from the start**: both suggestion collection keys default to `emptyList()` in
  the DTOs, and a MockWebServer test feeds `{"total": 0}` — `suggestions` key **absent** — asserting
  a clean empty parse. Hand-verified: removing the default makes that test fail.
- `households_role` was reworded from "Role (optional)" to "Role" — roles are now always picked from
  the constrained picker, so the "(optional)" framing was stale. The old free-text
  `households_member_vcard_uid` string was deleted (dead after the search-based picker).

**Strings:** 23 new/updated keys across all five locales (values, de/es/fr/it) — including the four
role labels, the search hints, the two suggestion-result messages (one count-parameterized), and the
accept/dismiss/section strings. `LocalesConsistencyTest` green.

**Tests (all hand-verified to fail against the broken behavior before restoring):**
- 6 new `ApiClientTest` MockWebServer tests (paths, methods, empty-body assertion, unwrapping, and
  the two T64 absent-key cases).
- `HouseholdsViewModelTest`: scan loads suggestions / empty scan renders empty state / scan failure /
  accept removes + reloads the household list / dismiss removes / suggestion member-name resolution.
- `HouseholdDetailViewModelTest`: name resolution on load, `updateMemberRole` round-trip (broken-repo
  call caught by `coVerify`), `suggestRelationships` count + no-new-suggestions messaging,
  <2-members no-op, search excluding existing members, blank-search no-op.

**CI gate green:** `./gradlew testDebugUnitTest`, `./gradlew lintDebug` (households module: "No
issues found"), `./gradlew assembleDebug`.

---

### Review pass, same day

A full review of the above found and fixed one real bug and a few test gaps:

- **"No role" would have sent `role: null` and gotten a 400.** The role picker's "No role" option
  passed `null` through to `HouseholdMemberInput(role = null)`, which Moshi serializes as
  `"role":null` — and the backend's PATCH binds `role` into a plain Go `string` (`memberUpdate{Role
  string}`, no pointer), which rejects an explicit JSON null with a 400. The whole clear-a-role path
  was broken. Fixed at two layers: the repository's `addMember`/`updateMember` now normalize
  `role.orEmpty()` before building the input (the backend's add DTO is `omitempty`, so `""` drops the
  field entirely), and `updateMemberRole` stores the normalized value. Hand-verified: reverting the
  normalization makes the two new repo tests fail. Web's equivalent always sends `role: ''`, never
  null — this closes the same latent pre-existing hole in the old free-text add dialog.
- **`HouseholdRepositoryImpl` had no test file at all.** New `HouseholdRepositoryImplTest`
  (Robolectric + in-memory Room, the `RelationshipEdgeRepositoryImplTest` pattern): the four new
  methods map/upsert/delegate correctly, accept failure leaves the cache untouched, and the null-role
  normalization is pinned on the wire via arg capture.
- **`formatSuggestionAddress` had no unit test.** New `HouseholdSuggestionFormatTest`: full-text
  precedence, component ordering/skipping, duplicate-kind handling, null/blank cases.
- **Missing error paths** now covered: `updateMemberRole` failure (error surfaces, updating flag
  resets, role unchanged), search failure (loading clears, no error surfaced — matching M21's
  search-policy), accept failure (error surfaces, suggestion retained, pending cleared).
- **Cleanups:** removed a dead `SEARCH_DEBOUNCE_MS` in `HouseholdsViewModel`'s companion, an unused
  `key` local in the suggestion list, scoped the accept/dismiss button disable to the in-flight card
  instead of every card (matching web's per-card pending), and made `displayNameFor`'s nickname
  fallback return `""` like M21's rather than a nullable nickname.
