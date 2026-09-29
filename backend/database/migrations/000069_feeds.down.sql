-- Reverses 000069 (issue #382). Destructive by nature: dropping the table
-- discards every feed credential, so any reader subscribed to a feed URL stops
-- working. There is no meaningful partial down -- the whole credential store is
-- the migration.

DROP INDEX IF EXISTS idx_feeds_user_revoked;
DROP TABLE feeds;
