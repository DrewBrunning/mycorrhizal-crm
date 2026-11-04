-- Rollback of 000089. Drops the column; stored ntfy tokens are lost and
-- ntfy publishes revert to anonymous.
ALTER TABLE notification_configs DROP COLUMN ntfy_token_encrypted;
