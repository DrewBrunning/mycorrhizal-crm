-- Terminal (permanent-until-human) failure state for outbound integrations
-- (INT-03 #466 hands off here; INT-04 #467 surfaces it).
--
-- The reliability gap this closes: a scheduled sync or a webhook retry loop that
-- keeps hammering a remote which will never recover on its own — a CardDAV
-- password changed six weeks ago, an address book deleted server-side — while
-- nothing tells the user their data stopped syncing. These columns record the
-- point at which retrying was abandoned, why, and when the failure began, so
-- the scheduler can stop attempting and the UI can show an actionable message.
--
-- All three are derived diagnostic state, not user data:
--   * webhook_deliveries.failed_permanently / terminal_reason — set when a
--     delivery hits a non-retryable HTTP status (401/403/404/410/4xx); the row
--     already carries the payload + error, this just records the disposition.
--   * {contact,calendar}_subscriptions.terminal_failure_at / terminal_reason —
--     NULL terminal_failure_at means "not terminal"; set by
--     SyncHealthFields.AdvanceForRun on a permanent failure, cleared on any
--     success or on a subscription edit.
-- Every existing row is simply "not terminal" until its next run classifies it,
-- so the columns default to 0 / '' / NULL and the down migration drops them
-- losslessly (same rationale as 000048's last_outcome).

ALTER TABLE webhook_deliveries ADD COLUMN failed_permanently INTEGER NOT NULL DEFAULT 0;
ALTER TABLE webhook_deliveries ADD COLUMN terminal_reason TEXT NOT NULL DEFAULT '';

ALTER TABLE contact_subscriptions ADD COLUMN terminal_failure_at DATETIME;
ALTER TABLE contact_subscriptions ADD COLUMN terminal_reason TEXT NOT NULL DEFAULT '';

ALTER TABLE calendar_subscriptions ADD COLUMN terminal_failure_at DATETIME;
ALTER TABLE calendar_subscriptions ADD COLUMN terminal_reason TEXT NOT NULL DEFAULT '';
