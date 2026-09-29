-- Private Atom feed credentials (issue #382, docs/adrs/0030-feeds-atom-emission.md
-- decision 6). A Feed is a per-feed URL secret that serves a read-only Atom view
-- of the owner's timeline; the plaintext token exists only in the create/rotate
-- response, so only its SHA-256 hex lives here (token_hash, unique).
--
-- Revocation is `revoked_at` and there is deliberately no `deleted_at`: like
-- ApiToken, a revoked row is kept for the audit trail but returns 404 and drops
-- out of the list. `entity_id` holds a contact VCardUID for kind='contact' and is
-- '' for kind='aggregate'; `detail` is 'headlines' (default) or 'full'. The
-- detail column default is a storage fallback only -- the application always sets
-- it explicitly, and the GORM model deliberately carries no `default:` tag (the
-- schema-parity default trap).

CREATE TABLE feeds (
    id               TEXT PRIMARY KEY,
    user_id          INTEGER NOT NULL,
    name             TEXT NOT NULL,
    kind             TEXT NOT NULL,
    entity_id        TEXT NOT NULL DEFAULT '',
    detail           TEXT NOT NULL DEFAULT 'headlines',
    token_hash       TEXT NOT NULL UNIQUE,
    last_accessed_at DATETIME,
    revoked_at       DATETIME,
    created_at       DATETIME NOT NULL,
    updated_at       DATETIME NOT NULL
);

CREATE INDEX idx_feeds_user_revoked ON feeds(user_id, revoked_at);
