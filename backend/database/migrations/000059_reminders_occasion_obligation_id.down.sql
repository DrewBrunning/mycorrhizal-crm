-- Rollback of 000059. occasion_obligation_id holds no durable user content
-- (a machine-synthesized link to a materialized reminder), so dropping it
-- loses nothing recoverable -- the reminder row itself, and the obligation it
-- points at, are untouched.
DROP INDEX IF EXISTS idx_reminders_occasion_obligation_id;
ALTER TABLE reminders DROP COLUMN occasion_obligation_id;
