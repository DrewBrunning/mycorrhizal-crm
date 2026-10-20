-- Optional, display-only group label on custom field definitions (issue #1730).
--
-- A group is just a shared label string clients bucket definitions by (e.g.
-- "Travel profile"); there is no group entity and nothing cascades on rename.
-- The column is named group_label because GROUP is an SQL keyword; the API
-- field is still `group`. DEFAULT '' means every existing row is ungrouped, so
-- no data is changed or lost.
ALTER TABLE field_definitions ADD COLUMN group_label TEXT NOT NULL DEFAULT '';
