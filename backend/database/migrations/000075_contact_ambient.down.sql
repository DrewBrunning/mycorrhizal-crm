-- Ambient contacts (issue #1733) -- rollback. Drops the explicit flag; any
-- contact the user had marked ambient reverts to scoring from logged activity
-- only. Destructive for the flag values set since the upgrade (nothing else
-- stores them).
ALTER TABLE contacts DROP COLUMN ambient;
