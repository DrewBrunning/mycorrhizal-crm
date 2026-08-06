-- Reverses 000013. Destructive by nature: the backfilled notification
-- deliveries, per-user channel configs/toggles, push subscriptions, and the
-- generated VAPID keys are all discarded. Down is the only lossy direction —
-- reminder delivery state has no way back to reminders.email_sent beyond the
-- backfill this reverses.

DROP TABLE notification_deliveries;
DROP TABLE push_subscriptions;
DROP TABLE notification_configs;
DROP TABLE server_settings;

ALTER TABLE users DROP COLUMN notify_ntfy;
ALTER TABLE users DROP COLUMN notify_gotify;
ALTER TABLE users DROP COLUMN notify_push;
