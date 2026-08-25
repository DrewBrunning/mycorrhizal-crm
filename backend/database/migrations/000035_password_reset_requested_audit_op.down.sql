-- Reverses 000035: narrows the operation CHECK back to the 000034 shape
-- (without 'password_reset_requested'). Destructive by construction, same as
-- 000034's own down migration: any row recorded with the now-unrepresentable
-- 'password_reset_requested' operation is discarded. Every other row,
-- including its chain hash, is preserved verbatim.

DROP TRIGGER IF EXISTS audit_events_no_update;

CREATE TABLE audit_events_old (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    operation TEXT NOT NULL CHECK (operation IN (
        'create', 'update', 'delete',
        'login', 'login_failed', 'register',
        'password_change', 'password_reset',
        'totp_enable', 'totp_disable', 'recovery_regenerate',
        'revoke', 'role_change'
    )),
    user_id INTEGER NOT NULL,
    before_snapshot TEXT,
    hash TEXT NOT NULL DEFAULT '',
    prev_hash TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

INSERT INTO audit_events_old (id, created_at, updated_at, entity_type, entity_id, operation, user_id, before_snapshot, hash, prev_hash)
SELECT id, created_at, updated_at, entity_type, entity_id, operation, user_id, before_snapshot, hash, prev_hash
FROM audit_events
WHERE operation != 'password_reset_requested';

DROP TABLE audit_events;
ALTER TABLE audit_events_old RENAME TO audit_events;

CREATE INDEX idx_audit_events_entity ON audit_events(entity_type, entity_id);
CREATE INDEX idx_audit_events_user ON audit_events(user_id);
CREATE INDEX idx_audit_events_created ON audit_events(created_at);

CREATE TRIGGER audit_events_no_update
BEFORE UPDATE ON audit_events
BEGIN
    SELECT RAISE(ABORT, 'audit_events is append-only: UPDATE is not allowed');
END;
