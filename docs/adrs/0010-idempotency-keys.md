# ADR 0010: One idempotency mechanism — a client-supplied `Idempotency-Key`

- **Status:** accepted
- **Date:** 2026-09-01
- **Depends on:** ADR 0004 (delete semantics), ADR 0008 (conditional writes — the sibling
  "retry safety for updates" mechanism)
- **Implements:** issue #459 (CON-04, idempotency audit). The milestone criterion is "Retrying
  mutations cannot silently duplicate data."
- **Contract behaviour** — DOC-03 (#488) documents it for API consumers; MAINT-02 (#491) treats a
  change to it as breaking.
- **Mirror:** #466 (INT-03) is the same reasoning for *outbound* operations we retry against
  someone else.

## Context

Retries are the normal operating mode of every client here: a phone on a flaky connection, a CardDAV
sync on a timer, a webhook delivery with its own retry loop, a user tapping a button twice. The
dangerous shape is the **ambiguous failure** — the write committed, the response was lost — because
the client cannot tell that from a write that never happened, so it retries. Whether that produces
one record or two (one email or two) is currently an unexamined per-endpoint property.

Some structure already exists and is reused rather than duplicated:

- `acquireJobLock` / `JobExecution` — at-most-once-per-window for scheduled work.
- `notificationDeliveryKey` — de-duplicates `(reminder, channel)` sends.
- Natural-key unique indexes on join tables — a duplicate membership add is a checked
  `409 ErrAlreadyExists`, not a second row.

Those cover timer-driven and structural duplication. What was missing is a stated, uniform answer for
**a client retrying an inbound mutation**.

## Decision

### Classify every mutation, three buckets

1. **Naturally idempotent** — `PUT` / `DELETE` on a known id, `PATCH` toggles, membership adds
   guarded by a unique index. Re-applying is a no-op or a checked `409`. **No new mechanism.** ADR
   0008's `If-Match` is the *update-conflict* guard for these; it is orthogonal to retry-safety.
2. **Naturally non-idempotent** — every `POST` that mints a new id or appends (notes, activities,
   life events, gifts, …), and anything with a side effect beyond the database (webhook, push,
   email). **These get the mechanism below.**
3. **Side-effecting beyond the database** — the sharpest case, a subset of (2): a row-level guard
   alone does not stop a second webhook. The mechanism handles this because it replays the *stored
   response without re-running the handler*, so `TriggerWebhooksAsync` never fires a second time.

### The one mechanism: `Idempotency-Key`

A client that cannot tolerate an ambiguous failure sends `Idempotency-Key: <opaque>` on its `POST`.
It is **one** thing, installed once on the whole authenticated group
(`middleware.IdempotencyMiddleware`), inert unless the request is a `POST` carrying the header — so
every current and future keyed `POST` is covered with no per-route wiring, and an un-keyed request is
byte-for-byte unaffected.

- **Claim** — `INSERT ... ON CONFLICT DO NOTHING` into `idempotency_keys`, keyed by
  `(user_id, idempotency_key)` (unique index — the claim is race-safe, no error-string sniffing).
- **First caller** — runs the handler, captures the response, and on a 2xx stores
  `(status, body, request_fingerprint)` with `state = completed`. A non-2xx **deletes** the pending
  row, so a corrected retry is allowed through (a failed create left nothing durable to replay).
- **Later caller, same key** —
  - fingerprint (hash of method + path + body) differs → `422 IDEMPOTENCY_KEY_REUSED`. The key no
    longer names one operation; the stored response would be the wrong answer, so it is **not**
    replayed.
  - `state = pending` (first call still in flight) → `409 IDEMPOTENCY_IN_PROGRESS`, retry shortly.
  - `state = completed` → the stored status + body are replayed verbatim, with an
    `Idempotency-Replayed: true` response header. The handler does not run, so no second row and no
    second side effect.

Why a client-supplied key and not, say, a server-computed body hash: it composes with retries the
client never told us about (a lost response looks identical to a lost request), it lets a client
deliberately scope "the same operation" (two genuinely-distinct creates with identical bodies get
distinct keys), and it is the conventional, discoverable answer API consumers already expect.

### Retention

`idempotency_keys` is transient: a key protects one operation across its retries, which happen within
seconds to minutes. `PurgeExpiredIdempotencyKeys` hard-deletes rows past
`IDEMPOTENCY_KEY_RETENTION_HOURS` (default 24; `<= 0` disables), on a 6-hour cron with a job lock —
the same shape as `webhook_delivery_purge` (#622). `response_body` can hold a copy of the created
entity, so the window also bounds that copy. User-scoped, hard-delete, swept by `DeleteUser`'s manual
cascade (CLAUDE.md backend trap #6) and pinned by `delete_cascade_coverage_test.go`.

### Documentation & drift

`backend/openapi.yaml` documents the `Idempotency-Key` request header (a shared
`components.parameters` entry) and the `409` / `422` responses on the `POST` endpoints in bucket (2);
the MAINT-02 baseline records them, so a later removal is a reviewable breaking-change diff.

## Consequences

- A keyed `POST` retried after an ambiguous failure yields exactly one record and one side effect.
- Double-submitting a create (same key) yields one contact, replayed.
- A retried keyed create that also fires a webhook fires it once — the replay path never re-enters
  the handler.
- Membership duplicate adds keep their existing `409 ErrAlreadyExists` (structural, unchanged).
- An un-keyed client is exactly as safe as before — the mechanism is opt-in, so a client that does
  not send the header still double-writes on a real double-submit. Making the header mandatory is a
  separate breaking-change decision, not taken here.
- Concurrency: `ON CONFLICT DO NOTHING` + the `_txlock=immediate` DSN (CLAUDE.md backend trap 9)
  serialise the claim; a concurrent retry that loses the claim race gets a `409` while the winner
  finishes, then replays. Pinned by a concurrent-retry test.
