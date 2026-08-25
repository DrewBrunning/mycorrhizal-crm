-- Widens the audit_events operation CHECK (issue #411) to add
-- 'password_reset_requested', distinct from the existing 'password_reset'
-- (which fires on successful confirm). Without this, a reset *request* --
-- the account-recovery entry point the issue asks to have audited -- has no
-- operation token to record under.
--
-- SQLite cannot ALTER a CHECK constraint, so this rebuilds the table exactly
-- like 000034 did, this time also copying the hash/prev_hash columns that
-- migration introduced (000034's own rebuild predated them, so it had
-- nothing to carry over). All existing rows, including their chain hashes,
-- are preserved verbatim.

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
        'revoke', 'role_change'
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
