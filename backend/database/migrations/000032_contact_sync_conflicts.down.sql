-- Surface CardDAV sync conflicts to the user (issue #395) — rollback.
ALTER TABLE contact_sync_links DROP COLUMN synced_values;
DROP TABLE contact_sync_conflicts;
