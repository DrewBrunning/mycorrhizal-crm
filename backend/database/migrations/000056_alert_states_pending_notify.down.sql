-- Rollback of 000056 (issue #973). A downgrade reverts the binary to code that
-- dispatches once and never retries, so the retry marker has no reader. Its
-- only content is "an undelivered raise is outstanding", already reflected in
-- the row's `alerting` state — dropping it loses no user data.
ALTER TABLE alert_states DROP COLUMN pending_notify;
