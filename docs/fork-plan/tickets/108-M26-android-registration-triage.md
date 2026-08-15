# M26 — Registration + circle/tag triage on Android

| | |
|---|---|
| **Platform** | Android |
| **Rating** | 2 — both real but low-frequency: one-time account creation, one-time legacy cleanup |
| **Size** | M — 2 new endpoints for registration, plus the triage UI (which needs none) |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | Nothing |
| **Status** | **DONE 2026-08-15** — see the landing note. |

Neither surface was marked deliberately-not-on-mobile at M8 sign-off (unlike admin user management,
which was), so both default to build-it per the parity target — filed together because both are
small, self-contained, and low-priority relative to the rest of this board.

## Scope

**Registration** (`RegisterPage.tsx:25-111`) — Android is currently sign-in only:
- Register a new account: username/email/password.
- Link from the login screen to registration (`LoginPage.tsx:104-106`).
- Forgot-password flow: request + confirm reset (`ForgotPasswordDialog.tsx`) — zero Android
  footprint; grouped here since it lives on the same screen as registration on web.

**Circle/Tag triage** (`CircleTagTriagePage.tsx`) — one-time cleanup tool for legacy free-text
circle strings inherited from the meerkat-crm fork:
- Collect distinct legacy-circle strings.
- Reclassify an item (circle / tag / skip), with inline rename before creating.
- Preview classification summary.
- Apply: create circle/tag entities and add members to them.

## Done when

- New-account creation and forgot-password both work from Android's login flow.
- Circle/tag triage is reachable and functionally equivalent to web's two-step (classify → preview
  → apply) flow.
- Hand-verified on-device: register a new account end-to-end; run triage against an instance with
  at least one un-triaged legacy circle string.
- New strings translated in all five locales.

---

## Implementation contract (added 2026-08-12)

Added in the readiness pass: this ticket came out of the [M8](89-M8-web-android-parity-audit.md)
parity audit, whose job was to prove a gap was real — not to specify the build. Endpoints, test
cases and the CI gate below close that difference.

### Endpoints

| Need | Route | In `ApiClient`? |
|---|---|---|
| Register | `POST /register` | **No** |
| Password strength | `POST /check-password-strength` | **No** |
| Circle/tag triage | `GET,PUT,DELETE /circles`, `/tags` (+ members) | Yes — all present |

Two new client methods for registration. **Triage needs none** — the whole circle/tag surface is
already in `ApiClient`, so that half is UI-only.

### Both halves are genuinely low-frequency

Account creation happens once; legacy circle/tag cleanup happens once. That is why this sits at the
bottom of the Android list — build it when the list above is clear, not because it is hard.

### Test cases

1. **Registration validation** — the password-strength response is surfaced *before* submission, and a
   weak password blocks submit rather than failing server-side.
2. **Duplicate email/username** surfaces the server's error against the right field.
3. **Success** lands the user authenticated, without a second manual login.
4. **Triage** — merging or deleting a legacy circle/tag updates membership and is confirmed first.

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
