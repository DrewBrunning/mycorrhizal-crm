-- Rollback of 000050. The index is pure derived access-path state; dropping it
-- only reverts the DB-01 integrity scan to its pre-index (full-scan) plan.

DROP INDEX IF EXISTS idx_contacts_user_vcard_uid_all;
