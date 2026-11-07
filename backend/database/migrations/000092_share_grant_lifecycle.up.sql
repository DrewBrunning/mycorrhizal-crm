-- Issue #2291, ADR 0042 decision 11: standing-share lifecycle.
--
-- 1. Widen the audit_events operation CHECK with 'accept', 'decline', 'expire'
--    and 'reconfirm' (the share_grant entity token needs no CHECK change).
--    SQLite cannot ALTER a CHECK constraint, so the table is rebuilt exactly as
--    000034/000035/000036/000070/000081 did; every existing row and chain hash
--    is preserved, indexes and the append-only trigger are recreated.
-- 2. share_grant_notifications: the recipient's in-app notices (a new pending
--    grant, a revoke). A row holds the sender id, the entity TYPE and a kind,
--    never an entity id, label or content; the sender username is joined at
--    read time. Hard delete: system-generated, removed by the lifecycle job
--    after the retention window and by the users FK cascade.
--
-- Additive for data: no existing row changes.

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
        'webauthn_register', 'webauthn_revoke',
        'restore', 'purge',
        'accept', 'decline', 'expire', 'reconfirm'
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

CREATE TABLE share_grant_notifications (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    from_user_id INTEGER NOT NULL,
    kind TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    read_at DATETIME,
    created_at DATETIME,
    updated_at DATETIME,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (from_user_id) REFERENCES users(id) ON DELETE CASCADE,
    CHECK (kind IN ('pending', 'revoked')),
    CHECK (entity_type IN ('contact', 'note', 'activity', 'circle'))
);

CREATE INDEX idx_share_grant_notifications_user_read ON share_grant_notifications(user_id, read_at);
CREATE INDEX idx_share_grant_notifications_from_user ON share_grant_notifications(from_user_id);
