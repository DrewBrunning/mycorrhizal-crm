-- Reverses 000012. Destructive by nature: dropping the columns discards every
-- gift URL and note the user has recorded. SQLite has supported
-- ALTER TABLE ... DROP COLUMN since 3.35 (the same mechanism 000010's down
-- migration uses for contacts.addresses_flat).

ALTER TABLE gifts DROP COLUMN url;
ALTER TABLE gifts DROP COLUMN notes;
