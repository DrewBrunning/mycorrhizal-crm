-- T69 — search
-- must find a contact by phone regardless of punctuation/grouping/country-code
-- differences between the query and the stored value.
--
-- T11 (000007) / T38 (000010) built FTS5 search over contacts but indexed
-- phone numbers only as the raw `phone` scalar with the default unicode61
-- tokenizer, which splits "(800) 555-1234" into tokens 800 / 555 / 1234 — a
-- query typed as 8005551234 prefix-matches none of them. The legacy
-- `applyContactSearch` LIKE path had the same gap (a substring cannot occur
-- inside a differently-punctuated value), and contacts_fts never indexed the
-- secondary Phones[] array entries at all.
--
-- Approach mirrors T38's: a denormalized, searchable phones_normalized TEXT
-- column on contacts, populated by Contact.BeforeSave (models/contact.go)
-- from EVERY Phones[] entry (not just the flat primary) as two tokens — the
-- full digit string and PhoneKey (T68)'s last-10-digits key when it differs —
-- so a query in either format matches a number stored in either format.
-- Backfilled here for rows that predate this migration and will never be
-- re-saved.
--
-- FTS5 cannot ALTER TABLE ADD COLUMN, so contacts_fts is dropped and
-- recreated with the new column, and its triggers are dropped/recreated the
-- same way — all derived data, rebuildable at any time via
-- services.RebuildSearchIndex. The notes/activities FTS tables and triggers
-- are untouched. Soft-delete and user-scoping behavior is unchanged from
-- 000007/000010.
--
-- The backfill deliberately mirrors Go's models.FlattenPhones (see
-- models/contact.go): per phones[] entry, its digits (SQLite has no regexp,
-- so a recursive CTE walks the string collecting ASCII digits) and, when the
-- entry has more than 10 digits, its last-10 key. Guarded by json_valid + a
-- COALESCE so a malformed/blank payload can never violate the NOT NULL column.

ALTER TABLE contacts ADD COLUMN phones_normalized TEXT NOT NULL DEFAULT '';

UPDATE contacts
SET phones_normalized = COALESCE((
    SELECT group_concat(tok, ' ')
    FROM (
        -- full digit string per entry
        SELECT digits AS tok
        FROM (
            SELECT (
                WITH RECURSIVE split(s, i, ch) AS (
                    SELECT p.value, 1, substr(p.value, 1, 1)
                    UNION ALL
                    SELECT s, i + 1, substr(s, i + 1, 1) FROM split WHERE i < length(s)
                )
                SELECT group_concat(ch, '')
                FROM split
                WHERE ch BETWEEN '0' AND '9'
            ) AS digits
            FROM json_each(contacts.phones) AS p
        ) WHERE digits <> ''
        UNION ALL
        -- last-10 key per entry, only when it differs from the digits
        SELECT substr(digits, -10) AS tok
        FROM (
            SELECT (
                WITH RECURSIVE split(s, i, ch) AS (
                    SELECT p.value, 1, substr(p.value, 1, 1)
                    UNION ALL
                    SELECT s, i + 1, substr(s, i + 1, 1) FROM split WHERE i < length(s)
                )
                SELECT group_concat(ch, '')
                FROM split
                WHERE ch BETWEEN '0' AND '9'
            ) AS digits
            FROM json_each(contacts.phones) AS p
        )
        WHERE length(digits) > 10
    )
), '')
WHERE json_valid(phones) AND phones <> '[]';

DROP TRIGGER IF EXISTS contacts_fts_au;
DROP TRIGGER IF EXISTS contacts_fts_ad;
DROP TRIGGER IF EXISTS contacts_fts_ai;
DROP TABLE IF EXISTS contacts_fts;

CREATE VIRTUAL TABLE contacts_fts USING fts5(
    firstname, lastname, nickname, email, phone, org, addresses_flat, phones_normalized,
    user_id UNINDEXED
);

CREATE TRIGGER contacts_fts_ai AFTER INSERT ON contacts BEGIN
    INSERT INTO contacts_fts(rowid, user_id, firstname, lastname, nickname, email, phone, org, addresses_flat, phones_normalized)
    VALUES (new.id, new.user_id, new.firstname, new.lastname, new.nickname, new.email, new.phone, new.org, new.addresses_flat, new.phones_normalized);
END;

CREATE TRIGGER contacts_fts_ad AFTER DELETE ON contacts BEGIN
    DELETE FROM contacts_fts WHERE rowid = old.id;
END;

CREATE TRIGGER contacts_fts_au AFTER UPDATE ON contacts BEGIN
    DELETE FROM contacts_fts WHERE rowid = old.id;
    INSERT INTO contacts_fts(rowid, user_id, firstname, lastname, nickname, email, phone, org, addresses_flat, phones_normalized)
    SELECT new.id, new.user_id, new.firstname, new.lastname, new.nickname, new.email, new.phone, new.org, new.addresses_flat, new.phones_normalized
    WHERE new.deleted_at IS NULL;
END;

INSERT INTO contacts_fts(rowid, user_id, firstname, lastname, nickname, email, phone, org, addresses_flat, phones_normalized)
SELECT id, user_id, firstname, lastname, nickname, email, phone, org, addresses_flat, phones_normalized
FROM contacts WHERE deleted_at IS NULL;
