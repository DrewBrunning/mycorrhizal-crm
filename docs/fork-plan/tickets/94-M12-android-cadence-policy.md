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
