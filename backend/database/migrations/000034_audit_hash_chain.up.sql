-- Tamper-evident audit log (issue #381, ASVS V7.3).
--
-- Two additions to audit_events:
--
--   1. A SHA-256 hash chain. Every row commits `hash(prev_hash || canonical
--      event content)`; VerifyAuditChain (models/audit_chain.go) recomputes
--      the whole chain and flags any insertion, deletion, reorder, or edit.
--      This is the integrity half of V7.3 — the audit table becomes
--      tamper-EVIDENT, not just append-only by convention.
--
--   2. A widened operation vocabulary. The old CHECK (create/update/delete)
--      only ever described entity CRUD; the auth/admin lifecycle events that
--      issue #381 also requires (login, password change, TOTP toggles,
--      API-token revoke, admin role change) legitimately need more tokens.
--
-- SQLite cannot alter a CHECK constraint, so this rebuilds the table. The
-- content is append-only and copied verbatim with ids preserved
-- (reach_out_suggestions.audit_event_id references them loosely). The
-- immutability trigger is dropped and re-created around the rebuild; the Go
-- chain backfill (models.RecomputeAuditChain, run at startup) drops and
-- re-creates it the same way, because before_snapshot may be encrypted at
-- rest (migration 000033_at_rest_encryption, issue #380) and SQL cannot
-- decrypt to hash.
-- Until that backfill runs, rows carry hash='' and the verifier reports them
-- as "chain backfill pending".

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

INSERT INTO audit_events_new (id, created_at, updated_at, entity_type, entity_id, operation, user_id, before_snapshot)
SELECT id, created_at, updated_at, entity_type, entity_id, operation, user_id, before_snapshot FROM audit_events;

DROP TABLE audit_events;
ALTER TABLE audit_events_new RENAME TO audit_events;

CREATE INDEX idx_audit_events_entity ON audit_events(entity_type, entity_id);
CREATE INDEX idx_audit_events_user ON audit_events(user_id);
CREATE INDEX idx_audit_events_created ON audit_events(created_at);

-- DB-level immutability (unchanged from 000016): audit rows may only be
-- inserted, never updated. The hash chain's sanctioned writers
-- (RecomputeAuditChain, the retention purge) drop and re-create this trigger
-- around their own updates, exactly like the migration itself just did.
CREATE TRIGGER audit_events_no_update
BEFORE UPDATE ON audit_events
BEGIN
    SELECT RAISE(ABORT, 'audit_events is append-only: UPDATE is not allowed');
END;
