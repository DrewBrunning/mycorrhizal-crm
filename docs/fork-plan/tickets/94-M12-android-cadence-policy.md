# M12 — Cadence policy panel for Android

| | |
|---|---|
| **Platform** | Android |
| **Rating** | 5 — T19 cadence is a rating-5 capability on web; Android has none of it |
| **Size** | M–L — 5 new endpoints plus screen, ViewModel, repository and route; the feature does not exist at all |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | Nothing — the T19 backend already exists and serves web today |
| **Status** | TO BE DONE |

Cadence/relationship-health (T19) has **zero Android footprint** — no screen, no ViewModel, no
repository, no route. `grep -rli cadence android/` turns up only unrelated notification-scheduling
code. This isn't a wiring gap like notes/activities; the feature was never started on Android. It
also feeds [M11](93-M11-android-prep-view.md)'s health card and
[M10](92-M10-android-dashboard-composite.md)'s overdue-cadences widget, so landing this unblocks
real content in both.

## Scope (mirrors `CadencePanel.tsx`/`CadenceDialog.tsx`)

- Set/edit cadence policy on a contact: interval (days), qualifying-interaction-type checkboxes
  (per `91.10`/`Activity.Qualifying()` — a qualifying interaction resets the cadence clock, not
  completing an unrelated task; see `/CLAUDE.md`'s domain notes).
- Delete a cadence policy.
- Health readout: overdue/on-track status, next-due date, last-interaction date.
- New `CadencePolicyRepository` in `core/domain`, a `cadence` nav route, and a screen following the
  existing per-contact sub-resource pattern (`RemindersScreen`/`RelationshipsScreen` are reasonable
  templates for structure).

## Done when

- Create/edit/delete a cadence policy from a contact on Android, round-tripping correctly with web
  (edit on one platform, see the health readout update on the other).
- Health readout matches web's overdue/on-track/next-due/last-interaction logic exactly — this is
  driven by shared backend logic, so it should be a read of the same computed fields, not a
  reimplementation.
- Hand-verified on-device: create a policy, log a qualifying activity, confirm the cadence clock
  resets (not on an unrelated completed task) per `/CLAUDE.md`'s domain note.
- New strings translated in all five locales.

---

## Implementation contract (added 2026-08-12)

Added in the readiness pass: this ticket came out of the [M8](89-M8-web-android-parity-audit.md)
parity audit, whose job was to prove a gap was real — not to specify the build. Endpoints, test
cases and the CI gate below close that difference.

### Endpoints

| Need | Route | In `ApiClient`? |
|---|---|---|
| List policies | `GET /cadence-policies` | **No** |
| One policy | `GET /cadence-policies/:id` | **No** |
| Create | `POST /cadence-policies` | **No** |
| Update | `PUT /cadence-policies/:id` | **No** |
| Delete | `DELETE /cadence-policies/:id` | **No** |
| Overdue list | `GET /cadence-policies/overdue` | Yes (`listOverdueCadences`) |

Five new client methods. The overdue call already exists because M10's dashboard widget uses it.

### Test cases

1. **Round-trip** — MockWebServer per verb: create sends the interval and qualifying-interaction
   types; update targets `/:id`; delete returns to the empty state.
2. **Health is read, never recomputed** — feed a response whose `overdue` flag contradicts what the
   local dates would imply, and assert the UI shows the **server's** verdict. This pins the ticket's
   "should be a read of the same computed fields, not a reimplementation" requirement, which nothing
   else would catch.
3. **Qualifying interactions** — the type checkboxes round-trip; an empty selection is preserved as
   empty rather than silently defaulting.
4. **Cadence resets on a qualifying interaction, not a completed task** (`91.10`) — assert the
   displayed next-due follows the server after logging a qualifying activity.

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

Landed on `feature/m12-cadence-policy`. The full cadence surface now exists on Android — screen,
ViewModel, repository, route, and the five new `ApiClient` methods — round-tripping against the
same T19 endpoints web uses.

**What shipped**

- **API client** — five new methods in `ApiClient` (`listCadencePolicies`, `getCadencePolicy`,
  `createCadencePolicy`, `updateCadencePolicy`, `deleteCadencePolicy`), each with a MockWebServer
  test pinning method + path + body. `listOverdueCadences` already existed. `OverdueCadence`'s
  `policy`/`health` were upgraded from `Map<String, Any?>` to the typed `CadencePolicy`/
  `CadenceHealth` (nothing read them as maps; the M10 widget will now get real types).
- **Models** — `CadenceHealth` (the server's verdict: `overdue_by > 0`, never recomputed
  locally), `CadencePolicy` (with embedded health, matching the backend's `CadencePolicyWithHealth`
  composite), `CadencePolicyInput` (`qualifying_types` empty = "all default-qualifying types
  count" — preserved as empty on round-trip, never defaulted), `CadencePoliciesResponse`,
  `CreateCadencePolicyResponse` (create is wrapped, update/get are raw — the documented backend
  asymmetry).
- **Repository** — `CadencePolicyRepository` in `core/domain`, online-first impl in `core/data`
  with a full-resync Room cache (`cached_cadence_policies`, migration `000014`→`000015`
  equivalent: Room version 14→15 with a hand-written `MIGRATION_14_15`, registered — a new table
  only, so nothing else changes; the destructive fallback stays off-limits per the
  `pending_interactions` rule).
- **Feature module** — new `feature/cadence` (screen + ViewModel + dialog), route
  `contacts/{contactId}/cadence`, and a "Cadence" entry point on `ContactDetailScreen` next to
  Relationships. The screen mirrors web's `CadencePanel`/`CadenceDialog`: interval pill
  ("every N days"), status row (overdue / on track / no-qualifying-interactions-yet), next-due +
  last-interaction captions, qualifying-type pills, edit/delete, and a create/edit dialog with a
  positive-interval validation and a checkbox per qualifying type. The add affordance (FAB +
  empty-state button) exists only in the no-policy state, matching web — creating a second policy
  is a server 409. `photo` is not offered as a qualifying type (it is the one globally
  non-qualifying type), matching web's token list.
- **Strings** — 28 `cadence_*` keys + a generic `action_edit` + `contact_cadence`, all in the
  five locale files (mirroring the web's existing translated `cadence` namespace). The `cadence_`
  namespace clears `LocalesConsistencyTest`'s no-byte-identical-to-English check.
- **Health is read, never recomputed** — the ticket's test case 2 is pinned two ways: a
  `CadenceHealth.isOverdue` that only reads `overdue_by`, and a screen test feeding a server
  verdict (`overdue_by: 3`) whose dates look on-track and asserting the server's "3 days overdue"
  renders. Hand-verified by breaking the verdict (forced `false`) and confirming the test fails,
  then restoring.

**CI gate**: `./gradlew testDebugUnitTest lintDebug assembleDebug --rerun-tasks` all green (1164
tasks). Two tests caught real bugs during the build: the dialog's lower checkboxes were
off-screen in Robolectric's small window (scrollable dialog column fixed it), and the Room schema
validation caught a `qualifyingTypes TEXT` vs `TEXT NOT NULL` mismatch in the migration (fixed to
match the generated schema exactly). Hand-verified per `/CLAUDE.md` on the two contract-critical
pins.

**Not verified on-device** — no device in the build env (same gap as M7/M21's caveats). The
ViewModel/screen/API tests plus the real-schema migration test are the substitute; the round-trip
with web ("edit on one platform, see the health readout update on the other") and the
"cadence resets on a qualifying interaction, not a completed task" on-device check from the
ticket's Done-when remain for the next on-device pass.

**Notes for later tickets** — M11 (prep view health card) and M10 (dashboard overdue widget) can
consume `CadencePolicy`/`CadenceHealth` directly now; `listOverdueCadences` already returns them
typed.

## Review pass (2026-08-14)

A full branch review (after the main rebase, against the merged M11 prep view) found and fixed
four real bugs and closed two drift hazards. All fixes are pinned by new tests that were
hand-verified to fail pre-fix (see `/CLAUDE.md`):

1. **Delete-confirmation dialog rendered unconditionally** ⚠ — `pendingDelete` was a `Boolean`
   but the dialog was gated with `pendingDelete?.let { AlertDialog(...) }`; a non-null `Boolean`
   makes `?.let` always run, so the "Delete cadence?" dialog permanently covered the screen. The
   panel-level Compose tests never rendered the full screen, so nothing caught it. Now
   `if (pendingDelete)`, pinned by a full-screen test ("the delete confirmation dialog is not
   shown until Delete is tapped") that fails against the old gate. The same review grepped for
   `?.let`-on-`Boolean` across the new code — the other two flags (`showDialog`, `intervalError`)
   were already correct.
2. **UTC-vs-device-zone date shift** — the health captions used `ZoneId.systemDefault()`, the
   exact bug class M11's review pass fixed in the prep view: web renders these calendar dates in
   UTC (`getUTCDate()`), so a device-zone renderer shows a different day for anyone west of UTC.
   Fixed by extracting a shared `DateFormat.formatTimestamp(iso, format)` in `core/model`
   (UTC + honors the format) used by **both** the prep view and the cadence panel, so the two
   screens cannot drift. Pinned by a `DateFormatTest` that forces a western device TZ and by a
   screen test rendering `2026-09-10T01:00:00Z` as "10 September 2026" (a device-zone renderer
   shows the 9th).
3. **The health captions ignored the user's `date_format`** — the cadence screen hardcoded
   `d MMMM yyyy` while web and M11's prep view honor the session preference, so the same
   next-due rendered differently across the two Android surfaces. `CadenceViewModel` now observes
   the session (`AuthRepository.observeSession()`, mirroring `PrepViewModel`) and the panel
   renders through `DateFormat.formatTimestamp`. Pinned by a ViewModel session-flow test and a
   screen test in `iso` format.
4. **Overdue/on-track colors deviated from web and the prep view** — the panel used `error` (red)
   / `primary` where web's `CadencePanel` uses `color="warning"` / `color="success"` (and M11's
   prep card uses `chanterelle` / `tertiary`), so "N days overdue" rendered red on the cadence
   screen and warning-orange on the prep view. Now `MycorrhizalColors.chanterelle` /
   `tertiary`.
5. **A failed create left a dead screen** — any write failure with no policy replaced the empty
   state with persistent error text and no way to retry. The screen now distinguishes the
   ViewModel's two error kinds (`errorRes` = fatal/missing contact, persistent body; `error` =
   transient, toasted) so a failed create returns to the retryable empty state. Pinned by a
   full-screen test asserting the empty state survives a 409 create.

Drift hazards closed: `DateFormat.formatTimestamp` is now the single timestamp→date renderer
(the prep view's private copy was deleted); `BriefingCadenceHealth`/`BriefingCadencePolicy`
(M11) and `CadenceHealth`/`CadencePolicy` (M12) carry cross-reference comments so the two
projections of the same backend object stay in sync. Removed the unused `cadence_loading` string
(Android uses `LoadingSkeleton`, not web's loading text); added a `photo`-exclusion note to
`CadenceQualifyingType` and a `cadenceTypeLabel`-sync note.

Testing added in the pass: `CadencePolicyRepositoryImplTest` (7 cases: mirror/full-resync/
tombstone-drop on list, create/update cache, delete removes, failed delete keeps the cached
row), four full-`CadenceScreen` tests (delete-dialog gating, delete-confirm→empty-state, FAB
visibility both ways, failed-create retryable), `DateFormat.formatTimestamp` unit tests (UTC pin
under a western TZ, eu/us/iso formats, offset timestamps, blank/unparseable), plus the
session-date-format ViewModel tests.

**CI gate** re-run after the review: `./gradlew testDebugUnitTest lintDebug assembleDebug
--rerun-tasks` all green (1164 tasks).
