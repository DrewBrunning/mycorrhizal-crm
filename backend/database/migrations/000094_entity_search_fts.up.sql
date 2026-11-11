-- Issue #2472 (part of #2174): full-text search over reusable entities
-- (organisations, places, pets; ADR 0043).
--
-- Derived data only: entities_fts is rebuildable from live `entities` rows at
-- any time (services.RebuildSearchIndex). Nothing in `entities` is rewritten.
--
-- Indexed text is entities.name only: entities.core is encrypted at rest, so a
-- trigger sees only ciphertext, and custom field values are never indexed.
--
-- Keyed by a stored entity_id, NOT by rowid: entities.id is a TEXT UUID on a
-- rowid table whose rowids VACUUM may renumber. Consequently the delete in the
-- triggers scans the (small, per-user) index by entity_id; acceptable at this
-- project's scale.
--
-- Soft delete: both the INSERT and UPDATE triggers carry the `deleted_at IS NULL`
-- guard (the migration-000059 rule), so a soft-deleted or merged-away entity is
-- never indexed and a restore (an UPDATE) re-indexes it.

CREATE VIRTUAL TABLE entities_fts USING fts5(
    name,
    entity_id UNINDEXED,
    user_id UNINDEXED,
    kind UNINDEXED
);

CREATE TRIGGER entities_fts_ai AFTER INSERT ON entities BEGIN
    INSERT INTO entities_fts(name, entity_id, user_id, kind)
    SELECT new.name, new.id, new.user_id, new.kind
    WHERE new.deleted_at IS NULL;
END;

CREATE TRIGGER entities_fts_ad AFTER DELETE ON entities BEGIN
    DELETE FROM entities_fts WHERE entity_id = old.id;
END;

CREATE TRIGGER entities_fts_au AFTER UPDATE ON entities BEGIN
    DELETE FROM entities_fts WHERE entity_id = old.id;
    INSERT INTO entities_fts(name, entity_id, user_id, kind)
    SELECT new.name, new.id, new.user_id, new.kind
    WHERE new.deleted_at IS NULL;
END;

-- Backfill the entities that already exist.
INSERT INTO entities_fts(name, entity_id, user_id, kind)
SELECT name, id, user_id, kind FROM entities WHERE deleted_at IS NULL;
