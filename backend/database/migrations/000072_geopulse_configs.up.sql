-- GeoPulse location-history connection config (issue #160, ADR 0033), one row
-- per user. Same exception as immich_configs/paperless_configs: the base URL +
-- API token are genuinely per-user-global (each user may run their own GeoPulse
-- instance), so they do not belong in a per-contact table.
--
-- The API token is stored ENCRYPTED via services/credential_crypto.go
-- (AES-256-GCM, key derived from JWT_SECRET_KEY) — never plaintext. A changed
-- JWT_SECRET_KEY makes the stored token undecryptable and the user must
-- re-enter it, exactly like calendar passwords.
--
-- GeoPulse's own user id (needed by its photo-search URL) is NOT stored: it is
-- discovered on demand from GET /api/users/me, so there is nothing for the user
-- to mis-enter and nothing to go stale.
--
-- One row per user, but the table soft-deletes, so the unique index must be
-- PARTIAL (WHERE deleted_at IS NULL) per T26: a soft-deleted row still occupies
-- every unique index it is in and a plain one would block re-connecting after
-- removing the connection. Mirrors 000025.

CREATE TABLE geopulse_configs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    deleted_at DATETIME,
    user_id INTEGER NOT NULL,
    base_url TEXT NOT NULL,
    api_key_encrypted TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX idx_geopulse_configs_user_id ON geopulse_configs(user_id)
    WHERE deleted_at IS NULL;
