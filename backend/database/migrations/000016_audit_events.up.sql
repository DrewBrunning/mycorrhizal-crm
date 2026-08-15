-- T18 — append-only audit log.
--
-- One row per create/update/delete of an audited entity, with a `before`
-- JSON snapshot for update/delete events so undo (updates only) can restore
-- the prior state. Immutable:
--
--   * The model (models/audit.go) has no update/delete receiver methods.
--   * The BEFORE UPDATE trigger below hard-rejects any UPDATE — rows can
--     never be changed after they are written.
--   * DELETE is guarded at the application layer: no model delete path
--     exists, and the only thing that ever deletes audit rows is the
--     retention purge job (AUDIT_RETENTION_DAYS) in services/
--     audit_purge_service.go. A hard no-DELETE trigger is deliberately not
--     used, because the purge legitimately deletes.
--
-- Hard-delete (not soft) per T26: audit rows are system-generated, never
-- user-authored.
--
-- `before_snapshot` carries redacted JSON (secret-typed fields stripped by
-- the recorder — see models/audit.go's deny-list) so the audit surface never
-- becomes a second copy of credentials.

CREATE TABLE audit_events (
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
CREATE INDEX idx_audit_events_entity ON audit_events(entity_type, entity_id);
CREATE INDEX idx_audit_events_user ON audit_events(user_id);
CREATE INDEX idx_audit_events_created ON audit_events(created_at);

-- DB-level immutability: audit rows may only be inserted, never updated.
CREATE TRIGGER audit_events_no_update
BEFORE UPDATE ON audit_events
BEGIN
    SELECT RAISE(ABORT, 'audit_events is append-only: UPDATE is not allowed');
END;
