-- Client-supplied idempotency keys for non-idempotent mutations (issue #459,
-- CON-04 — docs/adrs/0010-idempotency-keys.md).
--
-- The dangerous shape CON-04 addresses is the *ambiguous failure*: a POST that
-- minted a row succeeded on the server, but the response was lost in transit,
-- so the client (a phone on a flaky connection, a sync on a timer, a user
-- double-tapping) retries — and without a guard that produces a second row and
-- a second side effect (webhook, push).
--
-- A client that cannot tolerate that attaches `Idempotency-Key: <opaque>` to
-- the POST. The first request runs the handler and this table stores its
-- outcome; every later request with the same (user_id, key) replays the stored
-- response verbatim without re-running the handler — so exactly one row, one
-- webhook, one push, no matter how many times it is retried.
--
-- Lifecycle: immutable operational bookkeeping, hard-delete only, user-scoped
-- (swept by DeleteUser's manual cascade — CLAUDE.md backend trap #6). Rows are
-- short-lived: a TTL purge job hard-deletes them past
-- IDEMPOTENCY_KEY_RETENTION_HOURS (default 24), mirroring
-- webhook_delivery_purge (issue #622). `response_body` can hold a copy of the
-- created entity, so the retention window also bounds that copy the way the
-- webhook-delivery window bounds its payload.
--
-- `state` is 'pending' between the INSERT that claims the key and the UPDATE
-- that records the response; a concurrent retry that finds a 'pending' row is
-- told 409 (still processing) rather than being allowed to run the handler a
-- second time. A non-2xx outcome deletes the row (the handler did not commit a
-- durable effect worth replaying, and a corrected retry should be allowed
-- through).
--
-- `request_fingerprint` is a hash of method + path + body. A key replayed with
-- a *different* request is a client bug (the key no longer identifies "this
-- operation"); it is rejected 422 rather than silently replaying the wrong
-- stored response.

CREATE TABLE idempotency_keys (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id             INTEGER NOT NULL,
    idempotency_key     TEXT NOT NULL,
    method              TEXT NOT NULL,
    path                TEXT NOT NULL,
    request_fingerprint TEXT NOT NULL,
    state               TEXT NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'completed')),
    response_status     INTEGER NOT NULL DEFAULT 0,
    response_body       TEXT NOT NULL DEFAULT '',
    created_at          DATETIME NOT NULL,
    updated_at          DATETIME NOT NULL
);

-- The natural key: one row per (user, client key). The unique index is what
-- makes the claim-the-key INSERT race-safe — a concurrent retry's INSERT
-- fails here and falls through to the replay path.
CREATE UNIQUE INDEX idx_idempotency_keys_user_key ON idempotency_keys(user_id, idempotency_key);

-- Drives the TTL purge job.
CREATE INDEX idx_idempotency_keys_created_at ON idempotency_keys(created_at);
