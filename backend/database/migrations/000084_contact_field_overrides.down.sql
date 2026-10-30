-- Reverses 000084. Lossy by nature: every per-contact field override and the
-- account hide-empty default are discarded (display preferences only; no card
-- or contact data is touched).
ALTER TABLE users DROP COLUMN hide_empty_fields;
DROP TABLE contact_field_overrides;
