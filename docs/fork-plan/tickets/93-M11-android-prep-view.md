# M11 — Prep view (N2) for Android

| | |
|---|---|
| **Platform** | Android |
| **Rating** | 5 — N2 is a rating-5 capability on web; this is porting it, not a new design |
| **Size** | M — 1 endpoint, but a whole new screen |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | Nothing — the backend N2 endpoint(s) already exist and serve web today |
| **Status** | IMPLEMENTED, AWAITING ON-DEVICE VERIFICATION (2026-08-14) |

`/contacts/:id/prep` (`PrepViewPage.tsx`) has **zero** Android footprint — not even a placeholder
route in `MycorrhizalApp.kt`'s nav graph. It's the single largest capability gap this audit found:
the "what do I need to know before I talk to this person" briefing is exactly the kind of thing
someone reaches for on their phone right before a call or a visit, more than at a desk.

## Scope (mirrors `PrepViewPage.tsx:120-326`)

- Cadence health card: overdue-by / on-track / next-due / last-interaction.
- Open agenda items list (Conversation Agenda entries not yet marked discussed).
- Last interaction + recent notes.
- Relationships list, click-through to the other party's contact record.
- Life events list.
- Upcoming reminders list.
- Upcoming dates (birthday/anniversary, "in N days").
- Entry point: add to `ContactDetailScreen`'s header/menu (web reaches it from
  `ContactHeader.tsx:369,486`) and to the nav route `contacts/{contactId}/prep`.

## Done when

- All seven sections above present and populated from the real backend data (not stubbed).
- Reachable from the contact detail screen the same way web reaches it.
- Relationship/life-event entries navigate to their target contact, matching web.
- Hand-verified on-device against a contact with cadence, agenda items, notes, relationships, life
  events, and reminders all populated, per `/CLAUDE.md`'s workflow section.
- New strings translated in all five locales.

---

## Implementation contract (added 2026-08-12)

Added in the readiness pass: this ticket came out of the [M8](89-M8-web-android-parity-audit.md)
parity audit, whose job was to prove a gap was real — not to specify the build. Endpoints, test
cases and the CI gate below close that difference.

### Endpoints

| Need | Route | In `ApiClient`? |
|---|---|---|
| The briefing | `GET /contacts/:id/briefing` | **No.** Add `getBriefing`. |

One endpoint. N2's backend does all the assembly; the Android side is a read.

### Test cases

1. **Empty history must not crash** — MockWebServer: a briefing whose history/collection fields are
   **absent** from the JSON parses, and the screen renders an empty state. This is not hypothetical:
   the identical bug took web's prep view into the ErrorBoundary for any contact with no history
   (`/CLAUDE.md` frontend trap #8). Assert against raw JSON with the key omitted, not a Kotlin object
   with an empty list — those are indistinguishable once decoded, which is exactly why web's test passed.
2. **State machine** — loading → success populates; a network failure yields the error state with a
   retry that re-issues the call.
3. **Health card** — the cadence/health block reads server-provided fields rather than recomputing
   locally (see [M12](94-M12-android-cadence-policy.md), which supplies it).

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

All seven sections ship, populated from the real `GET /contacts/:id/briefing` backend data, plus the
entry point and nav route:

- **Network**: `getBriefing(contactId)` added to `ApiClient`; new `Briefing.kt` models
  (`ContactBriefing`, `BriefingActivity`, `BriefingCadence(Policy/Health)`, `BriefingRelationship`,
  `BriefingUpcomingDate`). The six collection blocks use the same nullable-raw + computed-property
  normalization as `NotesPage`/`FieldDefinitionsResponse` (`/CLAUDE.md` trap #8), so absent/null/`[]`
  all decode to an empty list — the exact contract regression that white-screened web's `PrepViewPage`
  cannot recur here. The API-client test asserting this reads **raw JSON with the keys omitted**, not a
  Kotlin object.
- **ViewModel**: `PrepViewModel` (injects `ApiClient` directly — the same precedent as
  `DashboardViewModel`, since a briefing is a pure read-only composite with no local cache or write
  path). State machine: loading → success / error-with-retry. The empty-history briefing is *success*,
  never an error; a `contactId` of 0 is an empty-not-error state.
- **Screen**: `PrepViewScreen` renders header (avatar/name/subtitle/animal chip), cadence health card
  (reads server-provided `health.*` fields verbatim — never recomputes locally, per M12), agenda,
  last-interaction + recent notes, relationships (tap-through to `other_party_contact_id`'s detail),
  life events, upcoming reminders, upcoming dates with `in N days` chips. Every block shows its empty
  state when the source is absent.
- **Entry point**: `ContactDetailScreen`'s ⋮ menu — the M24 "coming soon" stub is replaced with
  `onViewPrep`; nav route `contacts/{contactId}/prep` added to `MycorrhizalApp.kt`. The share stub
  stays (M15).
- **i18n**: 21 new keys (`prep.*` + `action_retry`) in all five locales, translated from web's
  `prep.*`/`cadence.*` strings (French apostrophes escaped), key-parity verified programmatically.
- **Tests**: 4 new MockWebServer tests in `ApiClientTest` (full composite parse, absent-collection
  keys, explicit-null collections, 404 error) + 6 `PrepViewModelTest` cases. Hand-verified per
  `/CLAUDE.md`: breaking the nullable-raw pattern fails the explicit-null parse test; removing the
  VM's `error` update fails both the error-state and retry tests; restoring passes.
- **CI gate**: `./gradlew testDebugUnitTest lintDebug assembleDebug --rerun-tasks` — green.

The ticket's on-device hand-verify step is still outstanding — no device/emulator available in the
build environment.

---

## Review pass (2026-08-14, same branch)

A full review pass found and fixed one real bug plus several divergences and test gaps:

- **Timezone bug (real, would ship wrong dates):** the prep view's date helper formatted ISO
  timestamps in the *device* zone. Web reads `getUTCDate()/getUTCMonth()/getUTCFullYear()` — a
  stored instant like `2026-09-10T01:00:00Z` renders as `2026-09-10` on web everywhere, but a user
  west of UTC would have seen `2026-09-09` on Android for the same briefing. Fixed to UTC and
  documented. The cadence next-due / last-interaction and last-activity dates are calendar-ish
  values computed server-side; they render in the zone they were authored in.
- **Date-format preference now honored:** `PrepViewModel` observes the session (same pattern as
  `ContactDetailViewModel`) and the screen renders dates via the app's `DateFormat` util, so the
  user's `date_format` applies; the upcoming-dates block also renders yearless `--MM-DD` values as
  "25 December" instead of the raw `--12-25`.
- **Cadence card colors:** overdue now renders in the warning (chantarelle) color and on-track in
  the success (moss/tertiary) color, matching web — being overdue is a nudge, not an error.
- **Relationship rows** gained a trailing chevron when the other party has a contact to link to
  (web's "View" chip, adapted to the app's tap-with-chevron convention).
- **Life-event rows** no longer drop the description when `type` is missing.
- **Double-tap retry guard:** `load()` dedupes overlapping loads via an in-flight `Job` — a retry
  tapped twice fires one request, not two.
- **Test gaps closed:** a new Robolectric/Compose UI test (`PrepViewScreenTest`, 4 cases) pins that
  the screen actually renders all seven sections from a populated briefing, renders empty states
  for a bare one without crashing, and that relationship rows navigate only when there is a target
  contact. A new Playwright spec pins the same all-seven-sections case end-to-end against the real
  API (activity + note + cadence policy + agenda item + relationship edge + life event + reminder +
  yearless birthday), so the wire contract the Android client consumes is proven fully populated.
  Both hand-verified per `/CLAUDE.md` (broke the tap path → test failed; restored).
- **Gate re-run:** `testDebugUnitTest`/`lintDebug`/`assembleDebug --rerun-tasks` green; full
  Playwright suite 181/181 green against the dockerized all-in-one image; frontend vitest 707/707;
  backend `go test ./...` green.
