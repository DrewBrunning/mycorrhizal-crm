-- T11 — FTS5 full-text over
-- contacts, notes, and interactions (activities). Derived data: the virtual
-- tables + triggers are rebuildable from source at any time (services/
-- search_service.go RebuildSearchIndex), which is exactly why this is safe
-- post-alpha — a rebuild is a re-runnable index job, not a destructive
-- migration.
--
-- Each FTS table carries `user_id UNINDEXED` so a search is scoped to one
-- user without joining the base table (the highest-risk correctness issue in
-- the ticket: a search must never cross users).
--
-- Soft-delete handling: the base tables soft-delete (GORM sets deleted_at on
-- UPDATE, never DELETE), so the AFTER UPDATE trigger removes the old FTS row
-- and re-inserts ONLY when deleted_at IS NULL. A soft-deleted row therefore
-- drops out of the index; hard deletes are covered by the AFTER DELETE
-- trigger. This is what makes "a soft-deleted row is not findable" hold
-- without a deleted_at filter in every query.
--
-- FTS5 special-command note: the documented INSERT-into-table-with-'delete'
-- command is NOT reliably supported by the pure-Go SQLite driver this project
-- uses (glebarez/sqlite / modernc.org/sqlite — it returns "SQL logic error"
-- even for an exact-match delete, verified during T11). The triggers instead
-- use a regular `DELETE FROM <fts> WHERE rowid = ?`, which the driver handles
-- correctly and which behaves identically for the delete-then-reinsert
-- pattern.

CREATE VIRTUAL TABLE contacts_fts USING fts5(
    firstname, lastname, nickname, email, phone, org,
    user_id UNINDEXED
);

CREATE VIRTUAL TABLE notes_fts USING fts5(
    content,
    user_id UNINDEXED
);

CREATE VIRTUAL TABLE activities_fts USING fts5(
    title, description, location,
    user_id UNINDEXED
);

-- ---------------------------------------------------------------------------
-- contacts triggers
-- ---------------------------------------------------------------------------
CREATE TRIGGER contacts_fts_ai AFTER INSERT ON contacts BEGIN
    INSERT INTO contacts_fts(rowid, user_id, firstname, lastname, nickname, email, phone, org)
    VALUES (new.id, new.user_id, new.firstname, new.lastname, new.nickname, new.email, new.phone, new.org);
END;

CREATE TRIGGER contacts_fts_ad AFTER DELETE ON contacts BEGIN
    DELETE FROM contacts_fts WHERE rowid = old.id;
END;

CREATE TRIGGER contacts_fts_au AFTER UPDATE ON contacts BEGIN
    DELETE FROM contacts_fts WHERE rowid = old.id;
    INSERT INTO contacts_fts(rowid, user_id, firstname, lastname, nickname, email, phone, org)
    SELECT new.id, new.user_id, new.firstname, new.lastname, new.nickname, new.email, new.phone, new.org
    WHERE new.deleted_at IS NULL;
END;

-- ---------------------------------------------------------------------------
-- notes triggers
-- ---------------------------------------------------------------------------
CREATE TRIGGER notes_fts_ai AFTER INSERT ON notes BEGIN
    INSERT INTO notes_fts(rowid, user_id, content) VALUES (new.id, new.user_id, new.content);
END;

CREATE TRIGGER notes_fts_ad AFTER DELETE ON notes BEGIN
    DELETE FROM notes_fts WHERE rowid = old.id;
END;

CREATE TRIGGER notes_fts_au AFTER UPDATE ON notes BEGIN
    DELETE FROM notes_fts WHERE rowid = old.id;
    INSERT INTO notes_fts(rowid, user_id, content)
    SELECT new.id, new.user_id, new.content
    WHERE new.deleted_at IS NULL;
END;

-- ---------------------------------------------------------------------------
-- activities triggers
-- ---------------------------------------------------------------------------
CREATE TRIGGER activities_fts_ai AFTER INSERT ON activities BEGIN
    INSERT INTO activities_fts(rowid, user_id, title, description, location)
    VALUES (new.id, new.user_id, new.title, new.description, new.location);
END;

CREATE TRIGGER activities_fts_ad AFTER DELETE ON activities BEGIN
    DELETE FROM activities_fts WHERE rowid = old.id;
END;

CREATE TRIGGER activities_fts_au AFTER UPDATE ON activities BEGIN
    DELETE FROM activities_fts WHERE rowid = old.id;
    INSERT INTO activities_fts(rowid, user_id, title, description, location)
    SELECT new.id, new.user_id, new.title, new.description, new.location
    WHERE new.deleted_at IS NULL;
END;
