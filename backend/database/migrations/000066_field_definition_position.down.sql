-- Rollback of 000065 (issue #1210).
--
-- Position is derived display state, not user-authored content: dropping the
-- column loses only the arrangement, never a value. The list falls back to
-- ordering by (updated_at, id) once the column is gone.
--
-- SQLite refuses to drop a column referenced by an index (3.35+), so the
-- position index must go first. That is also why the ADD COLUMN is the first
-- statement of the up migration and the index the last.
DROP INDEX IF EXISTS idx_field_definitions_user_position;
ALTER TABLE field_definitions DROP COLUMN position;
