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

---

## Landing note (2026-08-15)

Implemented on `feature/m15-android-contact-sharing` (see the branch's own history).

**What shipped.** A new `feature/shares` Android module mirroring the web's `ContactSharesPage.tsx`,
`ShareContactDialog.tsx` and `AcceptContactShareDialog.tsx`:

- **Standalone shares screen** (`ContactSharesScreen`, route `shares`, drawer-reachable via a new
  `nav_shares` item using the `IosShare` icon) with Incoming/Outgoing tabs, per-share status label,
  and for pending incoming shares an Accept and a confirm-gated Decline.
- **Accept flow** (`AcceptContactShareDialog`): accept is preview-then-confirm, never a one-tap
  button — it calls `POST /contact-shares/:id/accept` (preview-only, does not change status), shows
  the row's duplicate-match chip and an add/update/skip choice, and only then calls
  `POST /contact-shares/:id/confirm`. "Update" is only offered when a duplicate was matched,
  matching the backend's refusal to decide automatically.
- **Share from contact detail** (`ShareContactScreen`, route `contacts/{contactId}/share?uid=…`,
  replacing the "coming soon" stub in `ContactDetailScreen`'s action menu): recipient picker from
  `GET /users/directory`, the same T9 field-section picker with its sensitivity foot-gun guard
  (sensitive sections locked behind a deliberate reveal confirmation; `include_sensitive` only true
  when revealed AND a sensitive section is selected), then `POST /contact-shares`. The contact's
  VCard UID is passed through navigation (the detail screen already has it), matching the web's
  `vcardUID`-as-prop — the screen never re-fetches the contact, so a failed re-fetch can't masquerade
  as "this contact can't be shared".
- **7 new ApiClient methods** (`listIncomingContactShares`, `listOutgoingContactShares`,
  `createContactShare`, `acceptContactShare`, `confirmContactShare`, `declineContactShare`,
  `getUserDirectory`) plus a `ContactShareRepository` (online-only, no Room mirror — shares are
  inherently online data, following the `MergeRepositoryImpl` thin-passthrough precedent).

**Test coverage.** 10 new MockWebServer tests in `ApiClientTest` (round-trip per route, incl. a
query-param test), 17 ViewModel tests across `ContactSharesViewModelTest` + `ShareContactViewModelTest`
(accept preview-then-confirm with the exact confirm payload asserted, the decline gate asserted
*not* to call the repository until `confirmDecline`, the include-sensitive gating, default
non-sensitive section selection, a missing-uid guard, and a partial-failure test proving a failed
list surfaces its error instead of masquerading as "empty"), plus 10 new Compose tests
(`AcceptContactShareDialogTest` — duplicate chip, update-only-with-duplicate, confirm disabled while
loading/on-error/without-a-row; `ContactSharesScreenTest` — pending incoming row renders its actions,
Decline opens a confirmation dialog that gates the repository call, confirming calls it once, and
Accept opens the preview dialog) and an entry-point test in `ContactDetailScreenTest` rendering the
FULL `ContactDetailScreen` with a mocked ViewModel to pin test case 4 (the share action is reachable
from a contact's own header menu and carries the contact's uid). The decline-gate screen test and the
update-only-with-duplicate dialog test were hand-verified to fail against the reintroduced bugs and
restored. ~72 new strings across all five locales; the `LocalesConsistencyTest`
key-set/placeholder/namespace checks are green.

**Deliberate choices.** (1) `ShareFieldSections` lives in `core:model` as a hardcoded mirror of
backend `models/field_selection.go` + web `EXPORT_FIELD_SECTIONS` (frontend trap #4 — a comment
notes it must stay in sync by hand). (2) The accept dialog's options are rendered as a set of
selectable `OutlinedButton`s rather than a dropdown, since there are at most three. (3) Share
`section` labels are new `shares_section_*` strings (Android had no existing export section-picker
labels — Android's VCF export is full-contact). (4) Declining keeps the sender's copy untouched
server-side (that is backend behavior, pinned by the existing controller tests) — the UI only gates
the call, it does not delete anything locally. (5) A failed incoming OR outgoing list fetch surfaces
its error rather than rendering the failed list as "empty" — the other list still renders (this is a
deliberate divergence from the web's `Promise.all`, which blanks both lists on any single failure).
(6) A single-user instance (empty user directory, no error) shows a dedicated "no one to share with"
empty state instead of an unusable form.

**Outstanding.** On-device hand-verification with a second account sharing in both directions (the
ticket's "Hand-verified on-device" gate) still needs a real device — CI-level verification (unit +
lint + assemble) is green here.
