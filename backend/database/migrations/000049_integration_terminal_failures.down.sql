-- Rollback of 000049 (INT-03 #466 / INT-04 #467). Every column is derived
-- diagnostic state rebuilt by the next sync run / webhook delivery, so dropping
-- them loses nothing durable.

ALTER TABLE calendar_subscriptions DROP COLUMN terminal_reason;
ALTER TABLE calendar_subscriptions DROP COLUMN terminal_failure_at;

ALTER TABLE contact_subscriptions DROP COLUMN terminal_reason;
ALTER TABLE contact_subscriptions DROP COLUMN terminal_failure_at;

ALTER TABLE webhook_deliveries DROP COLUMN terminal_reason;
ALTER TABLE webhook_deliveries DROP COLUMN failed_permanently;
