# T57 — Documented/stable bulk-import API for external clients

| | |
|---|---|
| **Rating** | 1–2 when filed — **re-scoped and built 2026-08-14** once the Android app (M1) became the concrete consumer it was waiting for |
| **Size** | M (contract docs + idempotency hardening + contract-pinning test) |
| **Depends on** | [T56](65-T56-bulk-contacts-import-flow.md), [T96](140-T96-import-duplicate-merge-review.md) (shipped the records endpoint this ticket formalizes) |
| **Status** | **DONE** (2026-08-14) |
| **Source** | v0.3.0 post-release testing, 2026-08-06 — raised alongside T56: "when we make an Android app, having an API for it that works will let us import the entire set of contacts outright" |

## Why this exists

T56 gives the in-app UI a way to bulk-import. This ticket is the separate question of whether the
import machinery should be a **documented, stable** API contract — something an external client (the
Android app, most concretely) could drive directly rather than a human clicking through the Data
Settings UI. That's a materially different bar: a UI-backing endpoint can change shape whenever the UI
does; an API a mobile client depends on needs versioning discipline and can't casually break.

**Not just a first-run flow.** The Android app needs this from two separate places: a first-run
"would you like to import your contacts?" prompt, and a standing "Import from contacts" entry point
in the app's own Data settings, so a user can re-trigger an import after adding new device contacts.
Relative to the API, that distinction shouldn't matter: one stable bulk-import contract serves both
call sites identically.

## The design pass — what the deferred questions resolved to (2026-08-14)

When filed, the open questions were: what auth a mobile client uses, whether the session-based
preview/confirm flow is the right shape for a mobile bulk-import UX, and how the version story works
alongside T8's OpenAPI coverage. The Android app (M1) is now real and answered all three in practice:

- **Auth** — JWT bearer (`Authorization: Bearer <jwt>`), the same token the web app's httpOnly cookie
  authenticates with; Android's `AuthInterceptor` already replays it. API tokens
  (`Authorization: Bearer mycorrhizal_<token>`) work identically for non-browser clients.
- **Session flow shape** — the existing upload → preview → confirm pipeline **was adopted as-is**.
  It is synchronous and per-user, which fits a single-user self-hosted instance's mobile batch UX;
  no async job/status API is warranted. The records endpoint serves both entry points through the
  same contract.
- **Versioning** — spec-first via T8: `backend/openapi.yaml` is the contract, operationIds are the
  stable identifiers clients code against (Android's `ApiClient` method names match them), and the
  OpenAPI response spotcheck keeps the documented schemas truthful. No separate version scheme;
  breaking changes are still pre-alpha-announced, they just get caught when they break the spec.
- **Repeatability** — `POST /contacts/import/records` is the standing bulk-import endpoint: a JSON
  batch of neutral Card/CRM records (1–500) run through the same preview pipeline as every other
  import path (validation, per-row duplicate detection + merge diff, within-batch detection),
  confirmed via the shared `POST /contacts/import/vcf/confirm`.
- **Retry safety (new engineering, not just documentation)** — a confirm that commits but whose
  response the client loses used to leave the session deleted, so a retry 404'd and the client had
  no choice but to re-upload (double-importing). Confirms are now **idempotent**: a consumed
  session's result is kept as a 15-minute tombstone and replayed verbatim on retry. See the landing
  note.

## What shipped (landing note, 2026-08-14)

### Backend — idempotent confirm (`services/import_session.go`)

- **Per-session confirm lock** — `importSessionData.confirmMu` serializes concurrent confirms of the
  same session so a race can never apply an import twice (pinned by
  `TestConfirm_ConcurrentReconfirms_ApplyExactlyOnce`: 8 goroutines, exactly one apply).
- **Result tombstone** — a successful confirm stores the `ImportResult` (plus owner + expiry) in a
  small `confirmedResults` map, then deletes the full session payload. `CleanupExpired` purges
  tombstones at the normal 15-minute session window. A retried confirm of a consumed session returns
  the **original result** as a no-op instead of 404ing; ownership and expiry are still enforced
  (another user's retry gets 404; an expired tombstone 404s again — both pinned by tests).
- **Contract:** once confirmed, a session is consumed. Retries replay the first result; to change
  decisions a client starts a new upload. Sessions remain in-memory only (lost on restart — a
  post-restart retry 404s and must re-upload, documented).
- The Android client needs **no changes**: the response shape (`ImportResult`) is unchanged, so a
  dropped confirm now just retries the same request.

### Contract docs

- `backend/openapi.yaml` — the two confirm endpoints document the idempotent-replay semantics; the
  records endpoint is documented as the stable external-client bulk-import contract. Fixed a real
  spec inaccuracy the new spotcheck caught: `ImportRowPreview.duplicate_match` / `merge_diff` /
  `batch_duplicate_of` are emitted as `null` when absent, so they're now `nullable` (`allOf`-wrapped
  for the `$ref` fields — OpenAPI 3.0.3 + kin-openapi reject `type: "null"` and `$ref` siblings).
- `docs/api-reference.md` — Import section gained `/contacts/import/records` and
  `/contacts/import/jscontact/upload`, plus a **"Bulk import contract (external clients)"** section:
  session lifecycle (upload → preview → confirm), idempotent confirm, retry guidance, batch limits,
  ownership/expiry guarantees.

### Contract pinning test (`openapi_spotcheck_test.go`)

Extended the T8 spotcheck to drive a real records upload → confirm → **replayed confirm** round-trip
against the real router and migrated schema, validating every response body against the spec and
asserting the replay is byte-identical. This is the stability guarantee: the documented bulk-import
contract now has a test that fails if the implementation drifts from it.

## Traps

- OpenAPI 3.0.3 + kin-openapi cannot express a nullable `$ref` via `$ref` siblings (`description`,
  `nullable` next to `$ref` are rejected at spec load) or `type: "null"` in `oneOf` (rejected for
  3.0). The working pattern is `nullable: true` wrapping `allOf: [$ref: ...]`.
- Changing `batch_duplicate_of` etc. to `omitempty` would have **broken the web frontend**:
  `ImportContactsDialog.tsx` tests `row.batch_duplicate_of !== null`, which is `true` for an absent
  key. The API genuinely emits `null`, both clients rely on it, so the spec documents null rather
  than the serialization changing.
- `sessionData.confirmed` and the tombstone are separate mechanisms: the per-session field blocks
  the concurrent double-apply *before* the session is deleted; the tombstone serves the sequential
  retry *after* deletion. Neither alone covers both (the concurrency test failed with 24 contacts
  when `sessionData.confirmed` was forgotten).

## Review pass (2026-08-14, post-commit)

- **Fixed a real panic:** a `records`-sourced session ID sent to `/contacts/import/confirm` (the CSV
  endpoint) used to panic with `index out of range` — records sessions carry nil `csvContacts`, which
  `Confirm`'s add/update branches index. The confirm endpoints are now type-scoped both ways, matching
  ConfirmVCF's existing guard: `/contacts/import/confirm` rejects records sessions (400), and
  `/contacts/import/vcf/confirm` already rejected CSV ones. Pinned by a hand-verified test (panic
  reproduced before the fix) and documented in `openapi.yaml` + `api-reference.md`.
- **Coverage gaps closed:** a concurrent ConfirmVCF test on the records path (the actual Android
  device-contacts path, 8 goroutines → exactly one apply), and a pin that a consumed session's
  *preview* still 404s (the tombstone's replay is confirm-scoped only).
- Concurrent confirm tests verified `-race`-clean across 10 runs.

## Done when

- `cd backend && go build ./... && go vet ./... && gofmt -l . && go test ./...` green.
- Re-confirming a consumed session returns the original result with no second import (service tests
  + the OpenAPI spotcheck's replay assertion).
- Both import endpoints documented as idempotent in `openapi.yaml`; the bulk-import contract written
  up in `docs/api-reference.md`.
