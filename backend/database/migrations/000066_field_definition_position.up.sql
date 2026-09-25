-- Custom field-definition display order (issue #1210).
--
-- field_definitions is the user's typed-property registry; until now the list
-- endpoint ordered by (updated_at, id), so the settings list was effectively
-- newest-first and a user had no way to choose the order their custom fields
-- appear in. This adds the same `position` column LinkFieldType already has
-- (000009) so the list can be ordered by the user's own arrangement.
--
-- DEFAULT 0 is the interim value every row gets at ADD COLUMN time; the
-- backfill below then assigns each user's rows a contiguous position. The
-- GORM tag carries default:0 to match (models/schema_parity_test.go).
ALTER TABLE field_definitions ADD COLUMN position INTEGER NOT NULL DEFAULT 0;

-- Backfill: every existing row is assigned a per-user contiguous position in
-- created_at order (id breaks ties), which is what the list already did
-- implicitly for a freshly-created field. There is no data loss — position is
-- derived display state, and a user who never reorders keeps their existing
-- relative order.
UPDATE field_definitions
SET position = (
    SELECT rn
    FROM (
        SELECT id AS fid,
               ROW_NUMBER() OVER (
                   PARTITION BY user_id
                   ORDER BY created_at ASC, id ASC
               ) - 1 AS rn
        FROM field_definitions
    ) ranked
    WHERE ranked.fid = field_definitions.id
);

CREATE INDEX idx_field_definitions_user_position
    ON field_definitions(user_id, position, id);
