-- Per-subscription sync-health state (issue #390).
--
-- contact_subscriptions and calendar_subscriptions already carry
-- last_synced_at / last_sync_status / last_sync_error — a single current-state
-- bit. That answers "is it broken right now" but not "since when", "how many
-- times in a row", or "when did it last actually work". This adds the
-- last-known-good shape the sync subsystem needs (the cross-subsystem
-- generalization is issue #427):
--
--   last_attempt_at            every run, success or failure
--   last_success_at            most recent run that succeeded
--   last_failure_at            most recent run that failed
--   consecutive_failures       runs failed in a row; 0 after any success
--   incident_first_failure_at  first failure of the current unbroken run of
--                              failures; NULL whenever consecutive_failures = 0
--   last_run_duration_ms       wall-clock duration of the last run
--   last_run_stats             JSON of the last run's counter struct
--                              (services.ContactSyncStats / CalendarSyncStats)
--
-- Purely additive. Existing rows get sensible values backfilled from the
-- single state bit they already have: a subscription whose last recorded sync
-- errored is treated as one failure deep into an incident that started at
-- last_synced_at (the honest floor — the true historical count was never
-- recorded), a successful one as a clean last_success_at.
--
-- last_run_stats is a JSON-in-TEXT display blob, not queried and not derived
-- from a nested model (CLAUDE.md backend trap 2 does not apply); it is kept
-- an identical column on both tables even though the two stat structs differ,
-- so the two sync paths stay shape-for-shape aligned.

ALTER TABLE contact_subscriptions ADD COLUMN last_attempt_at DATETIME;
ALTER TABLE contact_subscriptions ADD COLUMN last_success_at DATETIME;
ALTER TABLE contact_subscriptions ADD COLUMN last_failure_at DATETIME;
ALTER TABLE contact_subscriptions ADD COLUMN consecutive_failures INTEGER NOT NULL DEFAULT 0;
ALTER TABLE contact_subscriptions ADD COLUMN incident_first_failure_at DATETIME;
ALTER TABLE contact_subscriptions ADD COLUMN last_run_duration_ms INTEGER;
ALTER TABLE contact_subscriptions ADD COLUMN last_run_stats TEXT NOT NULL DEFAULT '';

ALTER TABLE calendar_subscriptions ADD COLUMN last_attempt_at DATETIME;
ALTER TABLE calendar_subscriptions ADD COLUMN last_success_at DATETIME;
ALTER TABLE calendar_subscriptions ADD COLUMN last_failure_at DATETIME;
ALTER TABLE calendar_subscriptions ADD COLUMN consecutive_failures INTEGER NOT NULL DEFAULT 0;
ALTER TABLE calendar_subscriptions ADD COLUMN incident_first_failure_at DATETIME;
ALTER TABLE calendar_subscriptions ADD COLUMN last_run_duration_ms INTEGER;
ALTER TABLE calendar_subscriptions ADD COLUMN last_run_stats TEXT NOT NULL DEFAULT '';

-- Backfill from the pre-existing single state bit.
UPDATE contact_subscriptions SET last_attempt_at = last_synced_at WHERE last_synced_at IS NOT NULL;
UPDATE contact_subscriptions SET last_success_at = last_synced_at WHERE last_synced_at IS NOT NULL AND last_sync_status = 'success';
UPDATE contact_subscriptions
   SET last_failure_at = last_synced_at,
       consecutive_failures = 1,
       incident_first_failure_at = last_synced_at
 WHERE last_synced_at IS NOT NULL AND last_sync_status = 'error';

UPDATE calendar_subscriptions SET last_attempt_at = last_synced_at WHERE last_synced_at IS NOT NULL;
UPDATE calendar_subscriptions SET last_success_at = last_synced_at WHERE last_synced_at IS NOT NULL AND last_sync_status = 'success';
UPDATE calendar_subscriptions
   SET last_failure_at = last_synced_at,
       consecutive_failures = 1,
       incident_first_failure_at = last_synced_at
 WHERE last_synced_at IS NOT NULL AND last_sync_status = 'error';
