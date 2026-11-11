-- Rollback of 000094. entities_fts is derived data, so dropping it loses
-- nothing; every entities row is untouched.
DROP TRIGGER IF EXISTS entities_fts_au;
DROP TRIGGER IF EXISTS entities_fts_ad;
DROP TRIGGER IF EXISTS entities_fts_ai;
DROP TABLE IF EXISTS entities_fts;
