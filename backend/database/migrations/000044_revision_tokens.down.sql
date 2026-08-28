-- Monotonic per-row revision tokens (issue #591, CON-01a — ADR 0006) — rollback.
--
-- The revision/etag columns are derived state, fully rebuildable from the
-- model hooks going forward: dropping them loses no user data, only the
-- concurrency tokens (which would be invalidated anyway the moment a
-- client wrote the row again after the downgrade). The etag columns on
-- notes/reminders are brand-new and empty for any row saved after the
-- upgrade, so dropping them is lossless there too.
ALTER TABLE contacts DROP COLUMN revision;
ALTER TABLE activities DROP COLUMN revision;
ALTER TABLE life_events DROP COLUMN revision;
ALTER TABLE notes DROP COLUMN revision;
ALTER TABLE reminders DROP COLUMN revision;

ALTER TABLE notes DROP COLUMN etag;
ALTER TABLE reminders DROP COLUMN etag;
