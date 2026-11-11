-- Rollback of 000093 (issue #2463): drops the per-row issue columns. Only the
-- itemized detail is lost; every run and its counts survive.
ALTER TABLE import_runs DROP COLUMN issue_count;
ALTER TABLE import_runs DROP COLUMN issues;
