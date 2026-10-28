-- Rollback of 000079 (issue #2001). Dropping the columns loses any start/end
-- date recorded since the upgrade that was not also present in metadata
-- (since/until, which 000079 never modified). Deliberate, like 000058.
ALTER TABLE relationship_edges DROP COLUMN start_date;
ALTER TABLE relationship_edges DROP COLUMN end_date;
