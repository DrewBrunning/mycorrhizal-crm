# M11 — Prep view (N2) for Android

| | |
|---|---|
| **Platform** | Android |
| **Rating** | 5 — N2 is a rating-5 capability on web; this is porting it, not a new design |
| **Size** | M — 1 endpoint, but a whole new screen |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | Nothing — the backend N2 endpoint(s) already exist and serve web today |
| **Status** | TO BE DONE |

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
