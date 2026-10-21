-- Rollback of 000074 (issue #1730).
--
-- group_label is display-only categorisation, not a value: dropping the column
-- loses only the grouping (definitions and their values are untouched), and
-- clients fall back to one flat "Custom fields" block. No index references it.
ALTER TABLE field_definitions DROP COLUMN group_label;
