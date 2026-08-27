-- Reverses 000038. Destructive by nature: dropping the columns discards the
-- per-subscription sync-health history (consecutive-failure counts, incident
-- start times, per-run stats). The single state bit (last_synced_at /
-- last_sync_status / last_sync_error) predates this migration and stays.
-- SQLite has supported ALTER TABLE ... DROP COLUMN since 3.35.

ALTER TABLE contact_subscriptions DROP COLUMN last_attempt_at;
ALTER TABLE contact_subscriptions DROP COLUMN last_success_at;
ALTER TABLE contact_subscriptions DROP COLUMN last_failure_at;
ALTER TABLE contact_subscriptions DROP COLUMN consecutive_failures;
ALTER TABLE contact_subscriptions DROP COLUMN incident_first_failure_at;
ALTER TABLE contact_subscriptions DROP COLUMN last_run_duration_ms;
ALTER TABLE contact_subscriptions DROP COLUMN last_run_stats;

ALTER TABLE calendar_subscriptions DROP COLUMN last_attempt_at;
ALTER TABLE calendar_subscriptions DROP COLUMN last_success_at;
ALTER TABLE calendar_subscriptions DROP COLUMN last_failure_at;
ALTER TABLE calendar_subscriptions DROP COLUMN consecutive_failures;
ALTER TABLE calendar_subscriptions DROP COLUMN incident_first_failure_at;
ALTER TABLE calendar_subscriptions DROP COLUMN last_run_duration_ms;
ALTER TABLE calendar_subscriptions DROP COLUMN last_run_stats;
