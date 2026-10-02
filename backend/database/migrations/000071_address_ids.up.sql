-- Stable per-address identifiers for the contact map (ADR 0031, issue #694).
--
-- `contacts.addresses` is a JSON column, not a table, so there is no column to
-- add: the flat ContactAddress element simply gains `id`, `coordinates` and
-- `sensitivity` keys (old rows decode the missing ones as empty, meaning "no
-- coordinate" and "normal"). What needs real work is `id`: the geocode route
-- POST /contacts/:id/addresses/:addressId/geocode keys on it, and the flat
-- model never had one, so every existing address must be given one here
-- rather than waiting for its contact to be re-saved.
--
-- The ID has to agree between the two copies of an address: the flat
-- `addresses` array and the full-fidelity `card.addresses` array (a flat entry
-- is derived 1:1 from a card entry, in order). If only the flat copy were
-- stamped, the next plain save would see the loaded card entry (no ID) and the
-- flat entry (ID) as different projections and replace the card entry with the
-- lossy flat one -- discarding address components that have no flat slot. So
-- the IDs are generated once into a temp scratch table and written to both columns.
--   * A card entry that already has an ID (an imported JSContact address) keeps
--     it, and the flat entry adopts it.
--   * The card is only stamped when it has exactly as many addresses as the flat
--     array (the only case where index-pairing is meaningful); otherwise the
--     card is left alone and the flat entries get fresh IDs.
--   * Rows whose addresses column is not a non-empty JSON array are untouched,
--     as are entries that already carry an ID (makes the migration idempotent).
--
-- Data preservation: only the new `id` key is added to each element; every
-- existing key, order and value is preserved. updated_at / revision are NOT
-- touched (a schema backfill is not a user edit, so no If-Match token or sync
-- etag changes). The contacts_fts triggers fire for the UPDATEs and re-index
-- identical content. No audit events are written.
--
-- Verified by database/migrate_address_ids_test.go.

CREATE TEMP TABLE _migration_address_ids (
    contact_id INTEGER NOT NULL,
    idx        INTEGER NOT NULL,
    adr_id     TEXT    NOT NULL,
    PRIMARY KEY (contact_id, idx)
);

INSERT INTO _migration_address_ids (contact_id, idx, adr_id)
SELECT c.id,
       a.key,
       COALESCE(
           CASE WHEN json_valid(c.card)
                 AND json_array_length(c.card, '$.addresses') = json_array_length(c.addresses)
                THEN NULLIF(json_extract(c.card, '$.addresses[' || a.key || '].id'), '')
           END,
           lower(
               substr(hex(randomblob(4)), 1, 8) || '-' ||
               substr(hex(randomblob(2)), 1, 4) || '-4' ||
               substr(hex(randomblob(2)), 2, 3) || '-' ||
               substr('89ab', abs(random()) % 4 + 1, 1) || substr(hex(randomblob(2)), 2, 3) || '-' ||
               substr(hex(randomblob(6)), 1, 12)
           )
       )
FROM contacts c, json_each(c.addresses) a
WHERE json_valid(c.addresses)
  AND json_type(c.addresses) = 'array'
  AND COALESCE(json_extract(a.value, '$.id'), '') = '';

-- Flat copy: stamp the id onto each element that lacked one.
UPDATE contacts
SET addresses = (
    SELECT json_group_array(
        CASE
            WHEN COALESCE(json_extract(e.value, '$.id'), '') <> '' THEN json(e.value)
            ELSE json_set(e.value, '$.id', m.adr_id)
        END
    )
    FROM (SELECT key, value FROM json_each(contacts.addresses) ORDER BY key) AS e
    LEFT JOIN _migration_address_ids m ON m.contact_id = contacts.id AND m.idx = e.key
)
WHERE id IN (SELECT contact_id FROM _migration_address_ids);

-- Card copy: stamp the same ids by position, only where the card is
-- index-paired with the flat array.
UPDATE contacts
SET card = (
    SELECT json_set(contacts.card, '$.addresses', json_group_array(
        CASE
            WHEN COALESCE(json_extract(e.value, '$.id'), '') <> '' THEN json(e.value)
            ELSE json_set(e.value, '$.id', m.adr_id)
        END
    ))
    FROM (SELECT key, value FROM json_each(contacts.card, '$.addresses') ORDER BY key) AS e
    JOIN _migration_address_ids m ON m.contact_id = contacts.id AND m.idx = e.key
)
WHERE json_valid(card)
  AND json_type(card, '$.addresses') = 'array'
  AND json_array_length(card, '$.addresses') = (
      SELECT COUNT(*) FROM _migration_address_ids m WHERE m.contact_id = contacts.id
  )
  AND id IN (SELECT contact_id FROM _migration_address_ids);

DROP TABLE temp._migration_address_ids;
