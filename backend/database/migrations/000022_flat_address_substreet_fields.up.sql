-- T79 (docs/fork-plan/tickets/123-T79-flat-address-projection-too-narrow.md)
-- The flat ContactAddress projection gained PO box / apartment / floor slots.
--
-- No schema change is needed: `contacts.addresses` is a JSON column and old
-- rows simply decode the missing keys as empty strings. What this migration
-- does is recover real data. Contacts imported from a VCF still hold
-- postOfficeBox/apartment/floor components in `card` -- the vCard adapters
-- parsed them correctly and only the flat projection dropped them (that is
-- the whole ticket). Re-deriving `contacts.addresses` (and therefore
-- `addresses_flat`) from `card` for existing rows pulls that stranded detail
-- back into the editable, searchable surface.
--
-- Safety:
--   * Only rows whose card actually carries one of the three sub-street kinds
--     are touched -- every other row's flat addresses is left byte-for-byte
--     alone. The card is the authoritative full-fidelity copy (the T75 merge
--     in models/contact_card_merge.go keeps it a superset of the flat
--     projection), so re-deriving flat from card is a superset-preserving
--     operation for exactly those rows, and projected fields agree between
--     the two sides on any synced row.
--   * The addresses derivation mirrors Go's contactAddressFromNeutral
--     (models/contact_record_reverse.go) field for field, and the
--     addresses_flat recompute mirrors FormatAddress/FlattenAddresses
--     (models/contact.go) with the new parts ordered between street and city.
--     The SQL backfill and the Go derivation must not silently diverge, for
--     the same reason migration 000010 documents for its own backfill.
--   * The FTS index needs no explicit rebuild: contacts_fts is maintained by
--     triggers (000007/000010) and the UPDATEs below fire them.
--   * Soft-deleted rows stay out of the index -- the 000010 AFTER UPDATE
--     trigger's `deleted_at IS NULL` guard applies to these UPDATEs too.
--   * No audit events are written: raw SQL bypasses GORM's BeforeSave hooks,
--     which is correct here -- a schema backfill is not a user action.

-- Re-derive the flat addresses JSON from the authoritative card for the rows
-- that actually have stranded sub-street data. type comes from contexts[0],
-- exactly like contactAddressFromNeutral.
UPDATE contacts
SET addresses = (
    SELECT json_group_array(
        json_object(
            'type',      COALESCE(NULLIF(json_extract(a.value, '$.contexts[0]'), ''), ''),
            'street',    COALESCE((SELECT json_extract(c.value, '$.value') FROM json_each(a.value, '$.components') c WHERE json_extract(c.value, '$.kind') = 'name'), ''),
            'city',      COALESCE((SELECT json_extract(c.value, '$.value') FROM json_each(a.value, '$.components') c WHERE json_extract(c.value, '$.kind') = 'locality'), ''),
            'region',    COALESCE((SELECT json_extract(c.value, '$.value') FROM json_each(a.value, '$.components') c WHERE json_extract(c.value, '$.kind') = 'region'), ''),
            'postal',    COALESCE((SELECT json_extract(c.value, '$.value') FROM json_each(a.value, '$.components') c WHERE json_extract(c.value, '$.kind') = 'postcode'), ''),
            'country',   COALESCE((SELECT json_extract(c.value, '$.value') FROM json_each(a.value, '$.components') c WHERE json_extract(c.value, '$.kind') = 'country'), ''),
            'pobox',     COALESCE((SELECT json_extract(c.value, '$.value') FROM json_each(a.value, '$.components') c WHERE json_extract(c.value, '$.kind') = 'postOfficeBox'), ''),
            'apartment', COALESCE((SELECT json_extract(c.value, '$.value') FROM json_each(a.value, '$.components') c WHERE json_extract(c.value, '$.kind') = 'apartment'), ''),
            'floor',     COALESCE((SELECT json_extract(c.value, '$.value') FROM json_each(a.value, '$.components') c WHERE json_extract(c.value, '$.kind') = 'floor'), '')
        )
    )
    FROM json_each(card, '$.addresses') AS a
)
WHERE json_valid(card)
  AND EXISTS (
      SELECT 1
      FROM json_each(card, '$.addresses') AS a,
           json_each(a.value, '$.components') AS c
      WHERE json_extract(c.value, '$.kind') IN ('postOfficeBox', 'apartment', 'floor')
  );

-- Recompute the searchable addresses_flat from the (now widened) addresses
-- JSON for the same rows. Two statements, not one: in a single UPDATE every
-- SET expression is evaluated against the pre-update row, so the second
-- column could not see the first's new value.
UPDATE contacts
SET addresses_flat = COALESCE((
    SELECT group_concat(flat, ' ')
    FROM (
        SELECT (
            SELECT group_concat(e.value, ', ')
            FROM json_each(json_array(
                NULLIF(trim(json_extract(a.value, '$.street')), ''),
                NULLIF(trim(json_extract(a.value, '$.pobox')), ''),
                NULLIF(trim(json_extract(a.value, '$.apartment')), ''),
                NULLIF(trim(json_extract(a.value, '$.floor')), ''),
                NULLIF(trim(json_extract(a.value, '$.city')), ''),
                NULLIF(trim(json_extract(a.value, '$.region')), ''),
                NULLIF(trim(json_extract(a.value, '$.postal')), ''),
                NULLIF(trim(json_extract(a.value, '$.country')), '')
            )) AS e
            WHERE e.value IS NOT NULL
        ) AS flat
        FROM json_each(addresses) AS a
    )
), '')
WHERE json_valid(addresses) AND addresses <> '[]'
  AND EXISTS (
      SELECT 1
      FROM json_each(card, '$.addresses') AS a,
           json_each(a.value, '$.components') AS c
      WHERE json_extract(c.value, '$.kind') IN ('postOfficeBox', 'apartment', 'floor')
  );
