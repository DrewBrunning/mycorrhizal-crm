-- Issue #593 — WebAuthn/passkey credentials (backend half of #560).
--
-- One row per (user, credential). A passkey is an ALTERNATIVE second factor to
-- TOTP: login demands either. Hard delete (no deleted_at), same shape as
-- recovery_codes / contact_sync_links: the row's identity IS its natural key
-- (user_id, credential_id), so a lingering soft-deleted row would only block
-- re-enrolling the same authenticator. Revocation history lives in
-- audit_events, not in a tombstone.
--
-- credential_id / public_key / aaguid are raw bytes (BLOB). transports is a
-- comma-joined list of the authenticator's transport hints. backup_eligible /
-- backup_state mirror the WebAuthn BE/BS flags — the library rejects an
-- assertion whose BE flag differs from the stored one, so they must persist.

CREATE TABLE webauthn_credentials (
    id TEXT PRIMARY KEY,
    user_id INTEGER NOT NULL,
    credential_id BLOB NOT NULL,
    public_key BLOB NOT NULL,
    attestation_type TEXT NOT NULL DEFAULT '',
    aaguid BLOB,
    sign_count INTEGER NOT NULL DEFAULT 0,
    transports TEXT NOT NULL DEFAULT '',
    backup_eligible INTEGER NOT NULL DEFAULT 0,
    backup_state INTEGER NOT NULL DEFAULT 0,
    name TEXT NOT NULL DEFAULT '',
    created_at DATETIME,
    last_used_at DATETIME,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX idx_webauthn_credentials_user_credential ON webauthn_credentials(user_id, credential_id);
CREATE INDEX idx_webauthn_credentials_credential_id ON webauthn_credentials(credential_id);

-- Widen the audit_events operation CHECK with the passkey enrollment/removal ops.
-- SQLite cannot ALTER a CHECK constraint, so the table is rebuilt exactly as
-- 000034/000035/000036 did; every existing row and chain hash is preserved.
DROP TRIGGER IF EXISTS audit_events_no_update;

CREATE TABLE audit_events_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    operation TEXT NOT NULL CHECK (operation IN (
        'create', 'update', 'delete',
        'login', 'login_failed', 'register',
        'password_change', 'password_reset', 'password_reset_requested',
        'totp_enable', 'totp_disable', 'recovery_regenerate',
        'revoke', 'role_change', 'two_factor_admin_reset',
        'webauthn_register', 'webauthn_revoke'
    )),
    user_id INTEGER NOT NULL,
    before_snapshot TEXT,
    hash TEXT NOT NULL DEFAULT '',
    prev_hash TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

INSERT INTO audit_events_new (id, created_at, updated_at, entity_type, entity_id, operation, user_id, before_snapshot, hash, prev_hash)
SELECT id, created_at, updated_at, entity_type, entity_id, operation, user_id, before_snapshot, hash, prev_hash FROM audit_events;

DROP TABLE audit_events;
ALTER TABLE audit_events_new RENAME TO audit_events;

CREATE INDEX idx_audit_events_entity ON audit_events(entity_type, entity_id);
CREATE INDEX idx_audit_events_user ON audit_events(user_id);
CREATE INDEX idx_audit_events_created ON audit_events(created_at);

CREATE TRIGGER audit_events_no_update
BEFORE UPDATE ON audit_events
BEGIN
    SELECT RAISE(ABORT, 'audit_events is append-only: UPDATE is not allowed');
END;
