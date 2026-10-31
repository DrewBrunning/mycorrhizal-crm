-- CardDAV sync scope (issue #1993) -- rollback. Dropping the scope reverts
-- every user to 'all', so the next CardDAV sync widens what is served. The
-- release note for the version that ships this migration says so.
DROP TABLE IF EXISTS carddav_sync_scope_circles;
DROP TABLE IF EXISTS carddav_sync_scopes;
