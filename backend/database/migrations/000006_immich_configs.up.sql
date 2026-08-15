-- T15/T16 — the Immich
-- connection config, one row per user. This is the §93.4 exception where a
-- small typed config table is acceptable: the Immich base URL + API key are
-- genuinely per-user-global (not per-contact), so they do not belong in
-- ExternalIdentity.metadata (which is per-identity). Mirrors
-- calendar_subscriptions' shape.
--
-- The API key is stored ENCRYPTED via services/credential_crypto.go
-- (AES-256-GCM, key derived from JWT_SECRET_KEY) — never plaintext. A changed
-- JWT_SECRET_KEY makes stored keys undecryptable, and callers must treat that
-- as "credentials need to be re-entered", exactly like calendar passwords.
--
-- One row per user (user_id UNIQUE) — but the table soft-deletes (removing a
-- connection must not forget its sync history), so the unique index must be
-- PARTIAL (WHERE deleted_at IS NULL) per T26: a soft-deleted row still
-- occupies every unique index it is in, and a plain unique index here would
-- block re-creating the connection after removing it.

CREATE TABLE immich_configs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    deleted_at DATETIME,
    user_id INTEGER NOT NULL,
    base_url TEXT NOT NULL,
    api_key_encrypted TEXT NOT NULL DEFAULT '',
    sync_enabled INTEGER NOT NULL DEFAULT 1,
    last_synced_at DATETIME,
    last_sync_status TEXT NOT NULL DEFAULT '',
    last_sync_error TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX idx_immich_configs_user_id ON immich_configs(user_id)
    WHERE deleted_at IS NULL;
