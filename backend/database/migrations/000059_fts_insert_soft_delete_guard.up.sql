-- FTS insert triggers must not index soft-deleted rows (issue #354, found by
-- the migration suite).
--
-- The AFTER UPDATE trigger and the initial backfill for every *_fts index have
-- always carried a `deleted_at IS NULL` guard, but the AFTER INSERT trigger
-- never did. The app's own soft-delete path is an UPDATE, so the guard on `au`
-- covered every live path and the gap was invisible — until a database is
-- loaded with an already-soft-deleted row (a data restore, or the
-- internal/schemafixture copy that populates a historical schema before an
-- upgrade). Such a row lands in the index and stays searchable after the
-- upgrade, breaking DEPLOY-02's "a soft-deleted contact must not be searchable"
-- invariant (internal/schemafixture TestUpgradeLeavesSearchConsistent).
--
-- Recreate all three INSERT triggers with the same guard the UPDATE trigger
-- uses, then purge any soft-deleted rows already sitting in the index.
DROP TRIGGER IF EXISTS contacts_fts_ai;
CREATE TRIGGER contacts_fts_ai AFTER INSERT ON contacts BEGIN
    INSERT INTO contacts_fts(rowid, user_id, firstname, lastname, nickname, email, phone, org, addresses_flat, phones_normalized)
    SELECT new.id, new.user_id, new.firstname, new.lastname, new.nickname, new.email, new.phone, new.org, new.addresses_flat, new.phones_normalized
    WHERE new.deleted_at IS NULL;
END;

DROP TRIGGER IF EXISTS notes_fts_ai;
CREATE TRIGGER notes_fts_ai AFTER INSERT ON notes BEGIN
    INSERT INTO notes_fts(rowid, user_id, content)
    SELECT new.id, new.user_id, new.content
    WHERE new.deleted_at IS NULL;
END;

DROP TRIGGER IF EXISTS activities_fts_ai;
CREATE TRIGGER activities_fts_ai AFTER INSERT ON activities BEGIN
    INSERT INTO activities_fts(rowid, user_id, title, description, location)
    SELECT new.id, new.user_id, new.title, new.description, new.location
    WHERE new.deleted_at IS NULL;
END;

DELETE FROM contacts_fts WHERE rowid IN (SELECT id FROM contacts WHERE deleted_at IS NOT NULL);
DELETE FROM notes_fts WHERE rowid IN (SELECT id FROM notes WHERE deleted_at IS NOT NULL);
DELETE FROM activities_fts WHERE rowid IN (SELECT id FROM activities WHERE deleted_at IS NOT NULL);
