-- Activity/LifeEvent ETag primitives (T12a, docs/fork-plan/tickets/14).
-- ETag exists only on Contact and ContactSubscription today; CalDAV clients
-- (T12b) need a per-resource ETag to sync and cache both Interactions
-- (activities) and LifeEvents. Additive column, safe pre-alpha while the
-- tables are empty.
ALTER TABLE activities ADD COLUMN etag TEXT;
ALTER TABLE life_events ADD COLUMN etag TEXT;

-- Backfill existing rows in the same migration, mirroring 000009's contacts
-- etag format (e-{id}-{updated_at_unix}, where life_events.id is the UUID
-- string PK) so pre-existing rows are immediately syncable/cacheable by
-- CalDAV clients rather than waiting for their next write.
UPDATE activities
SET etag = 'e-' || id || '-' || CAST(strftime('%s', updated_at) AS TEXT)
WHERE etag IS NULL OR etag = '';

UPDATE life_events
SET etag = 'e-' || id || '-' || CAST(strftime('%s', updated_at) AS TEXT)
WHERE etag IS NULL OR etag = '';
