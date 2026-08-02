-- Re-create the v1 untyped custom-field columns (the Go models no longer
-- read them, so this is purely for rollback syntax-completeness).

ALTER TABLE users ADD COLUMN custom_field_names TEXT DEFAULT '[]';
ALTER TABLE contacts ADD COLUMN custom_fields TEXT DEFAULT '{}';
