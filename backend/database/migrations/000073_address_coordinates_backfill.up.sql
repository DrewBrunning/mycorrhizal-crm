-- Backfill the flat contact-map fields (`coordinates`, `sensitivity`) from the
-- paired neutral `card.addresses[i]` entry (issue #1440).
--
-- ADR 0031 added `id`, `coordinates` and `sensitivity` to the flat
-- `contacts.addresses` elements and to `card.addresses` at the same time, but
-- migration 000071 backfilled only `id` for pre-existing rows. A contact
-- imported with a vCard/JSContact/CardDAV GEO before v1.4.0 therefore has its
-- coordinate on the neutral Card alone. A plain save now preserves that
-- coordinate (the T75 merge in models/contact_card_merge.go), but the flat
-- column stays empty until that save happens, so GET /contacts/map — whose SQL
-- pre-filter is `addresses LIKE '%"coordinates"%'` — never sees it. This
-- migration closes that gap on upgrade, without waiting for a re-save.
--
-- Same index-pairing guard 000071 established: a card is only paired with the
-- flat array when its `addresses` is a JSON array of exactly the same length,
-- the only case where position i in one corresponds to position i in the
-- other. Every other row is left byte-for-byte untouched. `id` is deliberately
-- NOT touched: 000071 already backfilled it under the same guard, so
-- re-stamping it here would be redundant (and could churn an ID an
-- ID-unaware editor set since).
--
-- Directional: only an EMPTY flat value adopts the card's; a flat value that
-- is present is user data and is never overwritten. updated_at / revision are
-- NOT touched (a schema backfill is not a user edit, so no If-Match token or
-- sync etag changes). The contacts_fts triggers fire for the UPDATEs and
-- re-index identical content. Idempotent: a second run finds no empty flat
-- value paired with a populated card value, so it updates nothing.
--
-- Verified by database/migrate_address_coordinates_test.go.

-- coordinates
UPDATE contacts
SET addresses = (
    SELECT json_group_array(
        CASE
            WHEN COALESCE(json_extract(e.value, '$.coordinates'), '') = ''
             AND COALESCE(json_extract(c.value, '$.coordinates'), '') <> ''
            THEN json_set(e.value, '$.coordinates', json_extract(c.value, '$.coordinates'))
            ELSE json(e.value)
        END
    )
    FROM (SELECT key, value FROM json_each(contacts.addresses) ORDER BY key) AS e
    JOIN (SELECT key, value FROM json_each(contacts.card, '$.addresses') ORDER BY key) AS c
      ON c.key = e.key
)
WHERE json_valid(addresses)
  AND json_type(addresses) = 'array'
  AND json_array_length(addresses) > 0
  AND json_valid(card)
  AND json_type(card, '$.addresses') = 'array'
  AND json_array_length(card, '$.addresses') = json_array_length(addresses)
  AND EXISTS (
      SELECT 1
      FROM json_each(contacts.addresses) AS fe
      JOIN json_each(contacts.card, '$.addresses') AS fc ON fc.key = fe.key
      WHERE COALESCE(json_extract(fe.value, '$.coordinates'), '') = ''
        AND COALESCE(json_extract(fc.value, '$.coordinates'), '') <> ''
  );

-- sensitivity
UPDATE contacts
SET addresses = (
    SELECT json_group_array(
        CASE
            WHEN COALESCE(json_extract(e.value, '$.sensitivity'), '') = ''
             AND COALESCE(json_extract(c.value, '$.sensitivity'), '') <> ''
            THEN json_set(e.value, '$.sensitivity', json_extract(c.value, '$.sensitivity'))
            ELSE json(e.value)
        END
    )
    FROM (SELECT key, value FROM json_each(contacts.addresses) ORDER BY key) AS e
    JOIN (SELECT key, value FROM json_each(contacts.card, '$.addresses') ORDER BY key) AS c
      ON c.key = e.key
)
WHERE json_valid(addresses)
  AND json_type(addresses) = 'array'
  AND json_array_length(addresses) > 0
  AND json_valid(card)
  AND json_type(card, '$.addresses') = 'array'
  AND json_array_length(card, '$.addresses') = json_array_length(addresses)
  AND EXISTS (
      SELECT 1
      FROM json_each(contacts.addresses) AS fe
      JOIN json_each(contacts.card, '$.addresses') AS fc ON fc.key = fe.key
      WHERE COALESCE(json_extract(fe.value, '$.sensitivity'), '') = ''
        AND COALESCE(json_extract(fc.value, '$.sensitivity'), '') <> ''
  );
