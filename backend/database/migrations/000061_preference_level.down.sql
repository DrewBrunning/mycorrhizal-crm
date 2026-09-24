-- Reverses 000061 (issue #246). Destructive by nature: dropping the column
-- discards every proficiency level a user has recorded. The option that holds
-- real data is the up direction; category/key/value remain.
--
-- SQLite has supported ALTER TABLE ... DROP COLUMN since 3.35, the same
-- mechanism 000029's down migration uses.

ALTER TABLE preferences DROP COLUMN level;
