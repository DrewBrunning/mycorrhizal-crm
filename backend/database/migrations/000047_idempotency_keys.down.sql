-- Rollback of 000047 (issue #459, CON-04). idempotency_keys holds only
-- transient replay bookkeeping — every row is reconstructable by the client
-- simply retrying without a cached response, so dropping the table loses no
-- durable user data, only the dedup window.
DROP INDEX IF EXISTS idx_idempotency_keys_created_at;
DROP INDEX IF EXISTS idx_idempotency_keys_user_key;
DROP TABLE IF EXISTS idempotency_keys;
