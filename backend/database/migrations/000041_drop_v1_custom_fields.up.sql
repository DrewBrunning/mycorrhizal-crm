-- Remove the v1 untyped custom-field columns that T6/T7 replaced with the
-- typed v2 system (field_definitions + field_values, added by 000033).
-- The Go code stopped reading these in the T6/T7 feature branch; this
-- migration cleans the schema so they are not orphaned baggage forever.
-- Requires SQLite 3.35.0+ (2021-03-12) for DROP COLUMN.

ALTER TABLE users DROP COLUMN custom_field_names;
ALTER TABLE contacts DROP COLUMN custom_fields;
