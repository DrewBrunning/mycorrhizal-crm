-- Reverses 000029. Destructive by nature: dropping the column discards every
-- preference note a user has recorded. SQLite has supported
-- ALTER TABLE ... DROP COLUMN since 3.35 (the same mechanism 000012's down
-- migration uses for gifts.url/gifts.notes).

ALTER TABLE preferences DROP COLUMN notes;
