-- Rollback of 000071. The flat `contacts.addresses` elements lose the keys the
-- contact map added (id, coordinates, sensitivity); the card copy is left
-- as-is (an unknown `id`/`coordinates`/`sensitivity` on a card address is
-- harmless to the older binary). Destructive by construction for any
-- coordinate or sensitivity the user set on the flat copy since the upgrade.
UPDATE contacts
SET addresses = (
    SELECT json_group_array(json_remove(e.value, '$.id', '$.coordinates', '$.sensitivity'))
    FROM (SELECT key, value FROM json_each(contacts.addresses) ORDER BY key) AS e
)
WHERE json_valid(addresses)
  AND json_type(addresses) = 'array'
  AND json_array_length(addresses) > 0;
