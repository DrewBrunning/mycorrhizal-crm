# M15 — Contact sharing (P1) on Android

| | |
|---|---|
| **Platform** | Android |
| **Rating** | 3 |
| **Size** | L — 7 new endpoints, and accept opens a per-row preview flow rather than a button |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | Nothing — P1's backend already exists and serves web today |
| **Status** | TO BE DONE |

`/shares` (`ContactSharesPage.tsx`) has zero Android footprint, confirmed by a repo-wide search —
not just the standalone page but also the entry point on a contact's own header
(`ContactHeader.tsx:374-379` → `ShareContactDialog.tsx`).

## Scope (mirrors `ContactSharesPage.tsx` + `ShareContactDialog.tsx`)

- View incoming shares list.
- View outgoing shares list.
- Accept an incoming share (preview then confirm).
- Decline an incoming share.
- Initiate an outgoing share from a contact's own detail screen ("Share this contact").

## Done when

- All five actions above work on Android and round-trip with web (share from web, accept on
  Android and vice versa).
- Entry point exists both as a standalone shares screen (drawer-reachable) and from
  `ContactDetailScreen`'s header/menu.
- Hand-verified on-device with a second test account, sharing in both directions.
- New strings translated in all five locales.

---

## Implementation contract (added 2026-08-12)

Added in the readiness pass: this ticket came out of the [M8](89-M8-web-android-parity-audit.md)
parity audit, whose job was to prove a gap was real — not to specify the build. Endpoints, test
cases and the CI gate below close that difference.

### Endpoints

| Need | Route | In `ApiClient`? |
|---|---|---|
| Incoming shares | `GET /contact-shares/incoming` | **No** |
| Outgoing shares | `GET /contact-shares/outgoing` | **No** |
| Offer a share | `POST /contact-shares` | **No** |
| Accept | `POST /contact-shares/:id/accept` | **No** |
| Confirm | `POST /contact-shares/:id/confirm` | **No** |
| Decline | `POST /contact-shares/:id/decline` | **No** |
| Pick a recipient | `GET /users/directory` | **No** |

Seven new client methods — the largest API surface of any ticket in this batch.

**Accept and confirm are two different things — resolved 2026-08-12**, and this shapes the UI more
than any other detail in this ticket:

- **`POST /contact-shares/:id/accept` is preview-only.** It runs the share's payload through the
  VCF/JSContact import pipeline and returns an `ImportPreviewResponse` with duplicate matches. It
  **does not change the share's status** (`controllers/contact_share_controller.go:235-242`).
- **`POST /contact-shares/:id/confirm` finalizes it**, taking per-row add-vs-update actions the
  recipient chose from that preview, and only then flips the share to accepted (`:275-284`).

So "accept" is not a one-tap action. It opens a **preview screen with a per-row choice** — create new
vs merge into an existing contact — and confirm submits those choices. Building accept as a single
button would skip the decision the backend deliberately refuses to make automatically.

### Test cases

1. **Round-trip** — MockWebServer per route.
2. **Accepting an incoming share** removes it from the incoming list and the accepted contact appears
   locally.
3. **Declining requires confirmation** — the repository is not called until the user confirms. A
   mis-tap that permanently declines a share is the failure mode worth a test.
4. **Entry point** — the share action is reachable from a contact's own header, not only from a
   shares list (the audit flagged the header entry point as missing).

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
