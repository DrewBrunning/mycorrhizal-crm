-- Rollback of 000059 (issue #354): restore the three INSERT triggers to their
-- pre-000059 (unguarded) form. The index purge the up migration performed is
-- not reversed — re-indexing a soft-deleted row would reintroduce the exact
-- bug this migration fixes, and the live app's UPDATE path removes such rows
-- anyway.
DROP TRIGGER IF EXISTS contacts_fts_ai;
CREATE TRIGGER contacts_fts_ai AFTER INSERT ON contacts BEGIN
    INSERT INTO contacts_fts(rowid, user_id, firstname, lastname, nickname, email, phone, org, addresses_flat, phones_normalized)
    VALUES (new.id, new.user_id, new.firstname, new.lastname, new.nickname, new.email, new.phone, new.org, new.addresses_flat, new.phones_normalized);
END;

DROP TRIGGER IF EXISTS notes_fts_ai;
CREATE TRIGGER notes_fts_ai AFTER INSERT ON notes BEGIN
    INSERT INTO notes_fts(rowid, user_id, content) VALUES (new.id, new.user_id, new.content);
END;

DROP TRIGGER IF EXISTS activities_fts_ai;
CREATE TRIGGER activities_fts_ai AFTER INSERT ON activities BEGIN
    INSERT INTO activities_fts(rowid, user_id, title, description, location)
    VALUES (new.id, new.user_id, new.title, new.description, new.location);
END;
