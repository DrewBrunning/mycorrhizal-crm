-- T38 — search must
-- index address fields.
--
-- T11 (000007) built FTS5 search over contacts but never indexed addresses:
-- contacts_fts covers firstname/lastname/nickname/email/phone/org only, so
-- searching "Clark St" found nothing for a contact whose home address is on
-- Clark St. The legacy `applyContactSearch` LIKE fallback had the same gap.
--
-- Approach (the ticket's "lowest-friction option", consistent with how the
-- rest of contacts_fts already works — flat columns, not JSON json_each
-- matching): a denormalized, searchable `addresses_flat` TEXT column on
-- contacts, populated by Contact.BeforeSave (models/contact.go, the same hook
-- that keeps the legacy Address scalar in sync) alongside a one-time backfill
-- here for rows that predate this migration and will never be re-saved.
--
-- FTS5 cannot ALTER TABLE ADD COLUMN, so contacts_fts is dropped and
-- recreated with the new column, and its triggers are dropped/recreated the
-- same way — all derived data, rebuildable at any time via
-- services.RebuildSearchIndex (the "safe to change post-alpha" argument of
-- the ticket). The notes/activities FTS tables and triggers are untouched.
--
-- Soft-delete handling is unchanged from 000007: the AFTER UPDATE trigger
-- re-inserts ONLY when deleted_at IS NULL, so a soft-deleted contact's
-- address drops out of the index. User scoping is unchanged too: user_id is
-- still carried UNINDEXED and every query still filters on it.
--
-- The backfill deliberately mirrors Go's FormatAddress (non-empty components
-- joined with ", ") and FlattenAddresses (addresses joined with a space),
-- guarded by json_valid + a COALESCE so a malformed/blank JSON payload can
-- never violate the NOT NULL column.

ALTER TABLE contacts ADD COLUMN addresses_flat TEXT NOT NULL DEFAULT '';

UPDATE contacts
SET addresses_flat = COALESCE((
    SELECT group_concat(flat, ' ')
    FROM (
        SELECT (
            SELECT group_concat(e.value, ', ')
            FROM json_each(json_array(
                NULLIF(trim(json_extract(a.value, '$.street')), ''),
                NULLIF(trim(json_extract(a.value, '$.city')), ''),
                NULLIF(trim(json_extract(a.value, '$.region')), ''),
                NULLIF(trim(json_extract(a.value, '$.postal')), ''),
                NULLIF(trim(json_extract(a.value, '$.country')), '')
            )) AS e
            WHERE e.value IS NOT NULL
        ) AS flat
        FROM json_each(contacts.addresses) AS a
    )
), '')
WHERE json_valid(addresses) AND addresses <> '[]';

DROP TRIGGER IF EXISTS contacts_fts_au;
DROP TRIGGER IF EXISTS contacts_fts_ad;
DROP TRIGGER IF EXISTS contacts_fts_ai;
DROP TABLE IF EXISTS contacts_fts;

CREATE VIRTUAL TABLE contacts_fts USING fts5(
    firstname, lastname, nickname, email, phone, org, addresses_flat,
    user_id UNINDEXED
);

CREATE TRIGGER contacts_fts_ai AFTER INSERT ON contacts BEGIN
    INSERT INTO contacts_fts(rowid, user_id, firstname, lastname, nickname, email, phone, org, addresses_flat)
    VALUES (new.id, new.user_id, new.firstname, new.lastname, new.nickname, new.email, new.phone, new.org, new.addresses_flat);
END;

CREATE TRIGGER contacts_fts_ad AFTER DELETE ON contacts BEGIN
    DELETE FROM contacts_fts WHERE rowid = old.id;
END;

CREATE TRIGGER contacts_fts_au AFTER UPDATE ON contacts BEGIN
    DELETE FROM contacts_fts WHERE rowid = old.id;
    INSERT INTO contacts_fts(rowid, user_id, firstname, lastname, nickname, email, phone, org, addresses_flat)
    SELECT new.id, new.user_id, new.firstname, new.lastname, new.nickname, new.email, new.phone, new.org, new.addresses_flat
    WHERE new.deleted_at IS NULL;
END;

INSERT INTO contacts_fts(rowid, user_id, firstname, lastname, nickname, email, phone, org, addresses_flat)
SELECT id, user_id, firstname, lastname, nickname, email, phone, org, addresses_flat
FROM contacts WHERE deleted_at IS NULL;
