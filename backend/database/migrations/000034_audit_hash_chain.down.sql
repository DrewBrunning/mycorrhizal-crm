-- Reverses 000034: drops the hash-chain columns and narrows the operation
-- CHECK back to entity CRUD (the 000016 shape). Destructive by construction:
-- the chain columns are dropped (hashes are unrecoverable) and any
-- auth/admin lifecycle rows (login, password change, 2FA toggles, token
-- revoke, role change, register) are discarded because the restored CHECK
-- cannot represent them. Entity CRUD audit history is preserved verbatim.

DROP TRIGGER IF EXISTS audit_events_no_update;

CREATE TABLE audit_events_old (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    operation TEXT NOT NULL CHECK (operation IN ('create', 'update', 'delete')),
    user_id INTEGER NOT NULL,
    before_snapshot TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

INSERT INTO audit_events_old (id, created_at, updated_at, entity_type, entity_id, operation, user_id, before_snapshot)
SELECT id, created_at, updated_at, entity_type, entity_id, operation, user_id, before_snapshot
FROM audit_events
WHERE operation IN ('create', 'update', 'delete');

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
