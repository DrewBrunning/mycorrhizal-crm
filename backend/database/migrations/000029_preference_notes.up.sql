-- One additive nullable column on `preferences`, same shape and rationale as
-- 000012's gifts.notes: free-text context beyond what Value already holds
-- (e.g. "Alcohol" as the value, "Doesn't drink alcohol" as the note, instead
-- of cramming both into Value). Purely additive: no backfill, no existing
-- data touched.

ALTER TABLE preferences ADD COLUMN notes TEXT;
