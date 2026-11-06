-- Rollback of 000090. Drops the standing-share tables and their indexes; the
-- one-time contact_shares table is untouched.
DROP INDEX IF EXISTS idx_share_blocks_blocked_user;
DROP TABLE IF EXISTS share_blocks;
DROP INDEX IF EXISTS idx_share_grants_live_unique;
DROP INDEX IF EXISTS idx_share_grants_entity_status;
DROP INDEX IF EXISTS idx_share_grants_from_user_status;
DROP INDEX IF EXISTS idx_share_grants_to_user_status;
DROP TABLE IF EXISTS share_grants;
