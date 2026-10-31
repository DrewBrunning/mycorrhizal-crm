-- CardDAV sync scope by circle (issue #1993, ADR 0041).
--
-- carddav_sync_scopes is the per-user singleton: mode is one of
-- all/include/exclude. A user with no row is mode 'all', which is exactly
-- today's behaviour and needs no backfill.
--
-- carddav_sync_scope_circles stores the selected circles by their stable UUID.
-- The same table serves include and exclude; mode says which meaning applies.
-- Circles hard-delete, so the join row cascades with the circle. A scope's
-- default_circle_id is nullable and set to NULL when that circle is deleted;
-- server data is never deleted or auto-widened by a circle delete.
--
-- Both tables are hard-delete, edge-shaped rows (backend trap #7). They hold
-- only ids and a mode, no PII, and are deliberately outside the account bundle
-- (ADR 0041).
CREATE TABLE carddav_sync_scopes (
    user_id INTEGER PRIMARY KEY,
    mode TEXT NOT NULL DEFAULT 'all' CHECK (mode IN ('all', 'include', 'exclude')),
    default_circle_id TEXT,
    created_at DATETIME,
    updated_at DATETIME,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (default_circle_id) REFERENCES circles(id) ON DELETE SET NULL
);
CREATE INDEX idx_carddav_sync_scopes_default_circle ON carddav_sync_scopes(default_circle_id);

CREATE TABLE carddav_sync_scope_circles (
    user_id INTEGER NOT NULL,
    circle_id TEXT NOT NULL,
    PRIMARY KEY (user_id, circle_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (circle_id) REFERENCES circles(id) ON DELETE CASCADE
);
CREATE INDEX idx_carddav_sync_scope_circles_circle ON carddav_sync_scope_circles(circle_id);
