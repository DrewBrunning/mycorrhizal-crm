-- Rollback of 000067 (issue #1260).
--
-- The uuid column is a new stable identity for the account bundle; dropping it
-- loses only that identity, never a user-authored value. SQLite refuses to drop
-- a column referenced by an index, so the partial unique indexes go first.
DROP INDEX IF EXISTS idx_reminder_completions_user_uuid;
DROP INDEX IF EXISTS idx_reminders_user_uuid;
DROP INDEX IF EXISTS idx_notes_user_uuid;

ALTER TABLE reminder_completions DROP COLUMN uuid;
ALTER TABLE reminders DROP COLUMN uuid;
ALTER TABLE notes DROP COLUMN uuid;
