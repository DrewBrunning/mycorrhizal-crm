-- A non-partial companion index for the DB-01 data-integrity scan
-- (services/data_integrity_service.go, run on a timer by
-- CheckDBIntegrityScheduled).
--
-- The existing idx_contacts_vcard_uid_user is PARTIAL:
--   ON contacts(user_id, vcard_uid) WHERE vcard_uid IS NOT NULL AND deleted_at IS NULL
-- The integrity checkers (checkRelationshipEndpoints, checkOrphanedContactRefs,
-- checkDanglingExternalRefs) LEFT JOIN contacts ON vcard_uid WITHOUT a
-- `deleted_at IS NULL` filter — on purpose: they must see soft-deleted contacts
-- to detect a confirmed edge / join row pointing at a tombstone (INV-D3/D7).
-- SQLite cannot use a partial index for a query that needs the excluded rows,
-- so every one of those joins fell back to a full scan of `contacts`
-- (~O(refs x contacts): ~16 s at 4,000 contacts, observed under CAP-01).
--
-- This plain index covers the (user_id, vcard_uid) equality probe including
-- soft-deleted rows. It is pure derived access-path state — the down migration
-- drops it losslessly. The partial UNIQUE index stays and keeps enforcing the
-- one-live-vcard-uid-per-user constraint.

CREATE INDEX IF NOT EXISTS idx_contacts_user_vcard_uid_all
    ON contacts(user_id, vcard_uid);
