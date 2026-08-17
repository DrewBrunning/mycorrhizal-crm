-- P2c — the Nextcloud / ownCloud (WebDAV) connection config, one row per
-- user. Same §93.4 exception as the other per-user-global integration config
-- tables: the server URL + username + app password are genuinely per-user-
-- global, so they do not belong in ExternalIdentity.metadata.
--
-- Only an app password is accepted (never the user's real account password —
-- app passwords are the revocable, scoped credential shape and are the hard
-- requirement this integration is built on). It is stored ENCRYPTED via
-- services/credential_crypto.go (AES-256-GCM, key derived from JWT_SECRET_KEY)
-- — never plaintext.
--
-- One row per user (user_id UNIQUE) — soft-deletes, so the unique index must
-- be PARTIAL (WHERE deleted_at IS NULL) per T26, exactly like 000006/000025/000026.

CREATE TABLE webdav_configs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    deleted_at DATETIME,
    user_id INTEGER NOT NULL,
    base_url TEXT NOT NULL,
    username TEXT NOT NULL DEFAULT '',
    app_password_encrypted TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX idx_webdav_configs_user_id ON webdav_configs(user_id)
    WHERE deleted_at IS NULL;
