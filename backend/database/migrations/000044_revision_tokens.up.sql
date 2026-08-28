-- Monotonic per-row revision tokens (issue #591, CON-01a — ADR 0006).
--
-- Optimistic concurrency needs a per-row token a client can read and echo
-- back in a conditional write. The old CardDAV/CalDAV etag was derived from
-- ID + UpdatedAt.Unix() — one-second resolution, so two updates inside the
-- same second produced an identical token. It was also json:"-" (REST clients
-- could never obtain it).
--
-- This migration adds a `revision` counter to every user-authored soft-delete
-- entity (ADR 0004's soft-delete class): contacts, activities, life_events,
-- notes, reminders. The model hooks increment it on every persisted write and
-- derive the etag string from it (`e-{id}-{revision}`, same shape as before —
-- see docs/adrs/0006-revision-token-schema.md). notes/reminders never had an
-- etag column at all, so they get one now, purely additively.
--
-- Backfill: existing rows' revision is parsed from their old etag's numeric
-- suffix (`e-{id}-{n}` -> n) so the counter starts ABOVE any historical token
-- and no old etag value can ever be reused (a reset-to-1 backfill would let
-- `e-{id}-1234` recur and trip a client still holding the pre-migration value).
-- Rows with no parseable etag default to 1 (the column default).

ALTER TABLE contacts ADD COLUMN revision INTEGER NOT NULL DEFAULT 1;
ALTER TABLE activities ADD COLUMN revision INTEGER NOT NULL DEFAULT 1;
ALTER TABLE life_events ADD COLUMN revision INTEGER NOT NULL DEFAULT 1;
ALTER TABLE notes ADD COLUMN revision INTEGER NOT NULL DEFAULT 1;
ALTER TABLE reminders ADD COLUMN revision INTEGER NOT NULL DEFAULT 1;

ALTER TABLE notes ADD COLUMN etag TEXT;
ALTER TABLE reminders ADD COLUMN etag TEXT;

-- The numeric suffix after the last '-' of the etag. rtrim(etag, '0123456789')
-- strips the trailing digit run, so the digit run starts at
-- length(rtrim(...)) + 1 — works for both numeric PKs (e-12-<n>) and UUID
-- string PKs (e-<uuid>-<n>), where a naive "second dash" parse would split the
-- UUID's own dashes. MAX(..., 1) keeps a genuinely zero/empty suffix at the
-- column default rather than resetting the counter to 0.
UPDATE contacts    SET revision = MAX(1, CAST(substr(etag, length(rtrim(etag, '0123456789')) + 1) AS INTEGER)) WHERE etag LIKE 'e-%-%';
UPDATE activities  SET revision = MAX(1, CAST(substr(etag, length(rtrim(etag, '0123456789')) + 1) AS INTEGER)) WHERE etag LIKE 'e-%-%';
UPDATE life_events SET revision = MAX(1, CAST(substr(etag, length(rtrim(etag, '0123456789')) + 1) AS INTEGER)) WHERE etag LIKE 'e-%-%';
