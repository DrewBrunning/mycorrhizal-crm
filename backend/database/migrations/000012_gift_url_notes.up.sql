-- T35 — two additive
-- nullable columns on `gifts`, found using the shipped T20b feature for real:
--
--   url  — a link to the thing itself, for an idea captured as "she mentioned
--           she liked this specific item" with a product page to reference
--           later. Validated `safeurl` on write (models/gift.go) and re-checked
--           client-side before it is used as an href.
--   notes — free-text context beyond what the gift is (sizing, where you saw
--           it, "check they still want this before buying"). Description stays
--           the one-line "what it is".
--
-- Purely additive: nullable TEXT columns, no backfill needed, no existing data
-- touched. Real production data exists as of v0.2.0-alpha-candidate, so the
-- down migration is the only lossy direction — it drops the two columns and
-- with them anything a user has typed into them.

ALTER TABLE gifts ADD COLUMN url TEXT;
ALTER TABLE gifts ADD COLUMN notes TEXT;
