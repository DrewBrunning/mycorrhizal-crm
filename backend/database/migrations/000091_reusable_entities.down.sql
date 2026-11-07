-- Rollback of 000091 (issue #2295, ADR 0043 decision 9).
--
-- Refuses, rather than silently deleting entity data, when any entity, link or
-- non-contact field definition exists: the guard below fails the migration with
-- the constraint name as the message. Delete (and purge) the entities first.
CREATE TEMP TABLE _migration_000091_guard (
    live_rows INTEGER NOT NULL CONSTRAINT "cannot roll back 000091: entities, links or non-contact field definitions still exist; delete them first" CHECK (live_rows = 0)
);
INSERT INTO _migration_000091_guard
SELECT (SELECT COUNT(*) FROM entities)
     + (SELECT COUNT(*) FROM contact_entity_links)
     + (SELECT COUNT(*) FROM field_definitions WHERE target <> 'contact');
DROP TABLE _migration_000091_guard;

DROP INDEX idx_field_definitions_user_key;
CREATE UNIQUE INDEX idx_field_definitions_user_key ON field_definitions(user_id, key);

DROP TABLE IF EXISTS dismissed_entity_pairs;
DROP TABLE IF EXISTS entity_deletion_snapshots;
DROP TABLE IF EXISTS contact_entity_links;
DROP TABLE IF EXISTS entities;
