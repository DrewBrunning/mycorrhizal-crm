-- Stable portable IDs for notes, reminders and reminder completions (issue
-- #1260, ADR 0028 Decision 3).
--
-- The account bundle (GET /api/v1/export/account, issue #1259) needs a stable
-- ID for every entity so a re-import is idempotent and a re-export is
-- comparable. Every other entity already has one: contacts carry a VCardUID,
-- the UUID-PK entities generate their own, and Activity carries a `uuid`
-- column. Note, Reminder and ReminderCompletion were the exceptions — they
-- were uint-only with no portable identity.
--
-- This migration adds a `uuid TEXT` column to all three and backfills a
-- UUIDv4 for every existing row. UUIDv4 is generated in SQL: SQLite has no
-- uuid() function, but randomblob() plus the version/variant nibbles below
-- produce a genuine v4 (xxxxxxxx-xxxx-4xxx-[89ab]xxx-xxxxxxxxxxxx). The
-- expression is repeated per statement because each UPDATE evaluates it once
-- per row (randomblob is non-deterministic).
--
-- Data preservation: the UPDATE touches every row but changes only the new
-- column — no existing value is read or rewritten. It does fire the
-- notes_fts_au trigger (migration 000007) for each notes row, which re-indexes
-- identical content, so the FTS index stays consistent. This migration does
-- not touch audit_events, so the append-only immutability trigger from #772 is
-- not in play. The backfill is verified by
-- database/migrate_note_reminder_uuid_test.go.
--
-- The natural key (user_id, uuid) is a PARTIAL unique index (WHERE deleted_at
-- IS NULL): all three tables soft-delete (gorm.Model), so a soft-deleted row
-- must never block re-creating a row with the same stable ID (CLAUDE.md
-- backend trap 7).

ALTER TABLE notes ADD COLUMN uuid TEXT;
ALTER TABLE reminders ADD COLUMN uuid TEXT;
ALTER TABLE reminder_completions ADD COLUMN uuid TEXT;

UPDATE notes
SET uuid = lower(
    substr(hex(randomblob(4)), 1, 8) || '-' ||
    substr(hex(randomblob(2)), 1, 4) || '-4' ||
    substr(hex(randomblob(2)), 2, 3) || '-' ||
    substr('89ab', abs(random()) % 4 + 1, 1) || substr(hex(randomblob(2)), 2, 3) || '-' ||
    substr(hex(randomblob(6)), 1, 12)
);

UPDATE reminders
SET uuid = lower(
    substr(hex(randomblob(4)), 1, 8) || '-' ||
    substr(hex(randomblob(2)), 1, 4) || '-4' ||
    substr(hex(randomblob(2)), 2, 3) || '-' ||
    substr('89ab', abs(random()) % 4 + 1, 1) || substr(hex(randomblob(2)), 2, 3) || '-' ||
    substr(hex(randomblob(6)), 1, 12)
);

UPDATE reminder_completions
SET uuid = lower(
    substr(hex(randomblob(4)), 1, 8) || '-' ||
    substr(hex(randomblob(2)), 1, 4) || '-4' ||
    substr(hex(randomblob(2)), 2, 3) || '-' ||
    substr('89ab', abs(random()) % 4 + 1, 1) || substr(hex(randomblob(2)), 2, 3) || '-' ||
    substr(hex(randomblob(6)), 1, 12)
);

CREATE UNIQUE INDEX idx_notes_user_uuid
    ON notes(user_id, uuid)
    WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_reminders_user_uuid
    ON reminders(user_id, uuid)
    WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_reminder_completions_user_uuid
    ON reminder_completions(user_id, uuid)
    WHERE deleted_at IS NULL;
