DROP TRIGGER IF EXISTS contacts_fts_au;
DROP TRIGGER IF EXISTS contacts_fts_ad;
DROP TRIGGER IF EXISTS contacts_fts_ai;
DROP TABLE IF EXISTS contacts_fts;

ALTER TABLE contacts DROP COLUMN addresses_flat;

CREATE VIRTUAL TABLE contacts_fts USING fts5(
    firstname, lastname, nickname, email, phone, org,
    user_id UNINDEXED
);

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

INSERT INTO contacts_fts(rowid, user_id, firstname, lastname, nickname, email, phone, org)
SELECT id, user_id, firstname, lastname, nickname, email, phone, org
FROM contacts WHERE deleted_at IS NULL;
