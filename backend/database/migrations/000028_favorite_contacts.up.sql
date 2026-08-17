-- Favorite contacts (issue #173).
--
-- A single additive boolean on contacts: a user marks a contact as a favorite
-- via dedicated POST /contacts/:id/favorite (and /unfavorite), and the flag
-- surfaces in the list filter (?favorites=true), the detail view, and a
-- dashboard block.
--
-- No backfill needed: the DEFAULT 0 makes every existing row a non-favorite.
-- No index needed: like `archived`, all reads are over a single user's rows,
-- and this table is small. Mirror of `archived`, which has none either.

ALTER TABLE contacts ADD COLUMN is_favorite INTEGER NOT NULL DEFAULT 0;
