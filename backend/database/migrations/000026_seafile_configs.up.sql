-- P2b — the Seafile connection config, one row per user. Same §93.4 exception
-- as immich_configs/paperless_configs: the server URL + API token are
-- per-user-global, so they do not belong in ExternalIdentity.metadata.
--
-- The API token is stored ENCRYPTED via services/credential_crypto.go
-- (AES-256-GCM, key derived from JWT_SECRET_KEY) — never plaintext. A changed
-- JWT_SECRET_KEY makes stored tokens undecryptable, and callers must treat
-- that as "credentials need to be re-entered".
--
-- One row per user (user_id UNIQUE) — soft-deletes, so the unique index must
-- be PARTIAL (WHERE deleted_at IS NULL) per T26, exactly like 000006/000025.

CREATE TABLE seafile_configs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    deleted_at DATETIME,
    user_id INTEGER NOT NULL,
    base_url TEXT NOT NULL,
    api_token_encrypted TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX idx_seafile_configs_user_id ON seafile_configs(user_id)
    WHERE deleted_at IS NULL;
