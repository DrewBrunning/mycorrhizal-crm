# ADR 0013: Outbound retry safety and permanent-failure terminal state

- **Status:** accepted
- **Date:** 2026-09-03
- **Depends on:** INT-01 (`backend/integrations` classification matrix, issue #464 — `Dispositions()`
  is the single transient/permanent table); INT-02 (issue #465, the `faults.Hook` failure-injection
  seams); ADR 0010 (idempotency keys — this is the same "a retry must not duplicate a side effect"
  reasoning applied to *outbound* operations instead of inbound ones); issue #390 (`SyncHealthFields`,
  the per-subscription last-known-good state this extends).
- **Implements:** issues #466 (INT-03 — retry-safe integration operations) and #467 (INT-04 — surface
  permanent failures), landed together in one PR because #466 explicitly "hands off" its terminal
  state to #467. `v0.6.9` gate: #539.
- **Feeds:** #488 (DOC-03 operator docs, `v0.6.11`) cites this for the engineering half; #428
  (state-transition alerting) keys off the `healthy`↔`failing` flips this makes observable.

## Context

CON-04 (#459) covered idempotency for *inbound* mutations — a client retrying against us. This is the
mirror: *outbound* operations we retry against someone else, where the same ambiguity applies in the
other direction. We sent a request, we did not get a response, and we cannot tell whether it landed.

Two things were missing:

1. **A stated position on which outbound operations are safe to retry**, and what protects the ones
   that are not. Retry machinery already existed in places — `ProcessWebhookRetries` every five
   minutes, `notificationDeliveryKey` de-duplicating `(reminder, channel)` deliveries — but nothing
   said *why* a retry there was sound, and a new outbound write could be added with no classification.

2. **A terminal state for permanent failures.** A transient failure should be retried quietly and
   never bother anyone; a permanent one (revoked credentials, a remote resource deleted) will never
   resolve on its own, so retrying is pure waste and staying quiet means the user's data silently
   stops syncing. This is the "silent data staleness" case from #464 — the worst failure mode this
   product has: *a CardDAV password changed six weeks ago; the sync has failed every hour since; the
   contacts on the phone are two months stale; nothing anywhere said so.*

The old webhook retry loop made both mistakes: it retried every non-2xx identically on a fixed
`[5m, 15m]` schedule with no jitter, ignored `Retry-After`, and only raised its `integration_failed`
event after burning the whole 3-attempt budget — including for a `401` that was never going to
become a `200`.

## Decision

### 1. Classify every outbound operation (`integrations.OutboundOperations()`)

Each write or side-effecting call this app makes to an external system gets a row in
`backend/integrations/retry.go` with an **idempotency class**:

- `naturally-idempotent` — a bare retry converges (a PUT to a known URL, a DELETE). No operation is in
  this class today; it exists for completeness.
- `conditionally-idempotent` — safe only with a precondition. **caldav-push-event**: the PUT carries
  the remote object's own UID and an `If-Match` ETag, so a replay updates the same object in place and
  cannot create a duplicate.
- `not-idempotent` — creates a new remote resource or has a real-world side effect (sends a message).
  Safe to retry *only* because a local delivery record keyed by the logical event recognizes the
  retry: **webhook-delivery** (one immutable event envelope whose `id` rides every attempt as an
  `Idempotency-Key` header), **notification-{ntfy,gotify,webpush}** and **email-send-{resend,smtp}**
  (a `sent` `NotificationDelivery` for `(reminder, channel)` suppresses any further send).

`knownOutboundWriteIntegrations` is the curated set of `Registry()` IDs that perform a remote write; a
new one that adds no `OutboundOperations()` row fails `TestOutboundOperationsCoverWriteIntegrations`,
the same discipline as `nonIntegrationClients`. The classification is rendered into the generated
INT-01 matrix doc.

### 2. Shared retry primitives — one place decides backoff, jitter, `Retry-After`, and "never retry a permanent status"

`integrations.RetryPolicy` (max attempts, exponential backoff, `MaxDelay` ceiling, `±JitterFrac`),
`DispositionForHTTPStatus` (composes `Dispositions()` for the statuses that map to a `FailureMode`,
plus explicit `{PermanentUntilHuman, Retryable:false}` for request-level 4xx — `400/405/409/422/501` —
and "unknown 4xx is permanent, unknown 5xx is transient"), `ClassifyHTTPStatus`, and
`ParseRetryAfter` (seconds or HTTP-date). Every retry loop uses these instead of hand-rolling its own
schedule. The transient/permanent *judgment* still lives once in `Dispositions()`; these are the
operational mechanics on top of it.

### 3. Webhook delivery honors the classification (`services/webhook_service.go`)

- Non-retryable status → **terminal on that attempt**: `webhook_deliveries.failed_permanently = 1`,
  `terminal_reason` = the `FailureMode` slug, `next_retry_at = NULL`, and `integration_failed` raised
  immediately rather than after the budget is spent.
- `429`/`503` → `next_retry_at` is never sooner than the `Retry-After` hint.
- Everything else transient → `webhookRetryPolicy` exponential backoff (base 5m ×3, ±20% jitter,
  capped at 6h), still bounded by `maxDeliveryAttempts` (3).
- Retry state stays in the `webhook_deliveries` row, so `ProcessWebhookRetries` re-scans after a
  restart — pending retries survive. (That job now spawns its retry goroutines through the tracked
  `webhookGoroutines` runner, matching the fan-out path.)

### 4. Sync terminal state (`SyncHealthFields` + `AdvanceForRun`)

`SyncHealthFields` gains `TerminalFailureAt *time.Time` (NULL ⇒ not terminal) and `TerminalReason`.
`AdvanceForRun` takes a `terminalReason` the caller derives from the run error via
`services.classifySyncFailure` — `""` for a transient failure, `"auth-expiry"` /
`"remote-resource-deleted"` for the two `PermanentUntilHuman` modes a sync can actually hit. On a
permanent failure it enters the terminal state, **frozen at first entry** (`TerminalFailureAt` answers
"when did this stop working" and must not move on subsequent permanent runs). **Any success clears
it** — that is the recovery signal, and it flips `ComputeSubsystemHealth` `failing`→`healthy`,
closing the incident for #428.

- `SyncAllCalendars` (the only scheduled sync — CardDAV contact sync is user-triggered) filters
  `terminal_failure_at IS NULL`: a terminal subscription is **not attempted**. An unbounded hourly
  retry against a permanently broken remote is a self-inflicted denial of service, on them and on our
  own job schedule.
- Recovery is explicit: editing the subscription (`Update{Calendar,Contact}Subscription` →
  `ClearTerminalState`) or a manual `POST .../sync` (which bypasses the scheduler filter) tries again;
  a manual sync that still fails re-sets the state and returns `terminal_reason` in the error's
  `details` so the user sees the actionable cause.

### 5. Surface it where the user looks, not only where an operator looks (INT-04 action 3)

- `CalendarSyncSettings` and the new `ContactSyncSettings` (CardDAV) panel render a prominent error
  `Alert` — a reason-specific actionable message plus a staleness line ("Last successful sync: 47 days
  ago", or "never synced successfully" to distinguish *stopped working* from *never worked*).
- The webhook list response carries a `delivery_health` rollup of the latest delivery, so a
  permanently-failed receiver shows a "Will not retry" badge next to the webhook itself, and each
  terminal delivery row is marked.

## Consequences

- A permanent failure now costs one failed run and one event, not a budget's worth, and it is
  visible in the web UI next to the thing it broke.
- A terminal subscription stops consuming scheduler time until a human acts. The trade-off: if a
  remote returns `401` transiently (misconfigured briefly), the user must re-trigger the sync rather
  than waiting for the next scheduled run. Judged acceptable — a real `401` is far more often
  permanent, and the manual retry is one click.
- `AdvanceForRun`'s signature changed (added `terminalReason`); it is still the single writer of every
  sync-health column.
- New migration `000049` adds `failed_permanently` + `terminal_reason` to `webhook_deliveries` and
  `terminal_failure_at` + `terminal_reason` to both subscription tables — all derived diagnostic
  state, defaulted, losslessly droppable (same rationale as `000048`).

## Verification

`integrations/retry_test.go` (backoff bounded + jittered + monotonic without jitter, `NextDelay`
honors `Retry-After`, `DispositionForHTTPStatus` table, `ParseRetryAfter`);
`integrations/matrix_test.go` + `int03_coverage_test.go` (every write integration classified, every
operation has a retry-safety test); `services/webhook_retry_safety_test.go` (permanent status →
terminal-now + one event + never re-picked-up; `Retry-After` honored; transient backoff bounded;
`Idempotency-Key` stable across a retry; a retry produces exactly one additional POST);
`services/sync_failure_behavior_test.go` (401 → terminal + local data untouched + `sync_failed`;
`SyncAllCalendars` skips a terminal subscription; recovery clears it; 503 is not terminal);
`models/sync_health_test.go` (`AdvanceForRun` terminal lifecycle — set once, frozen, cleared on
success). Hand-verified per CLAUDE.md: deleting the permanent-status early return in `deliverWebhook`
fails the "401 does not retry" test; deleting the `terminal_failure_at IS NULL` filter in
`SyncAllCalendars` fails the "scheduler skips a terminal subscription" test. Frontend:
`ContactSyncSettings.test.tsx`, `CalendarSyncSettings.test.tsx`, `WebhooksSettings.test.tsx`,
`utils/syncHealth.test.ts`.
