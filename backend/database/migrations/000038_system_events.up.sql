-- Structured operational-event model + system event timeline (issue #424,
-- companion to #425's correlation IDs).
--
-- system_events is the persisted, chronological record of *what happened to
-- the system* — application start/stop, migrations, scheduled jobs, sync
-- runs, notification dispatch, backup/restore drills — so an admin has one
-- place to answer "when did this start / what changed / what's failing" that
-- survives a restart. It is deliberately distinct from:
--   * audit_events  — who changed user-authored CRM data (that is user data;
--                     this is operational)
--   * job_executions — only the lock + last-run bookkeeping
--   * contact_sync_conflicts — a user-data event surfaced to one user
--
-- System-generated, hard-delete (no deleted_at): rows are removed only by the
-- retention purge (SYSTEM_EVENT_RETENTION_DAYS, default 30), mirroring
-- audit_events' lifecycle (CLAUDE.md backend trap 7). Read-only over the API,
-- admin-only (instance-wide, not user-scoped).
--
-- No hash chain (unlike audit_events): this is diagnostics, not a
-- tamper-evident legal record. No high-cardinality fields — no contact IDs,
-- no raw URLs; `detail` carries bounded values only (counts, subsystem
-- names). `error` is sanitized + length-capped by the emitter
-- (models.RecordSystemEvent -> logger.SanitizeLogField).
--
-- The event_type / severity / result CHECK vocabularies are mirrored by hand
-- in models/system_event.go and frontend/src/api/systemEvents.ts — a token
-- added in one place without the others is a silent INSERT failure or a
-- missing label (CLAUDE.md frontend trap 4; same pattern as audit_events
-- 000034/000035).
CREATE TABLE system_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME NOT NULL,
    occurred_at DATETIME NOT NULL,
    event_type TEXT NOT NULL CHECK (event_type IN (
        'application_started', 'application_stopped',
        'migration_started', 'migration_completed', 'migration_failed',
        'job_started', 'job_completed', 'job_failed',
        'sync_started', 'sync_completed', 'sync_failed',
        'notification_sent', 'notification_failed',
        'backup_completed', 'backup_failed', 'restore_test_completed',
        'integration_failed'
    )),
    severity TEXT NOT NULL DEFAULT 'info' CHECK (severity IN ('info', 'warn', 'error')),
    component TEXT NOT NULL DEFAULT '',
    operation TEXT NOT NULL DEFAULT '',
    duration_ms INTEGER,
    result TEXT CHECK (result IS NULL OR result IN ('success', 'failure', 'skipped')),
    correlation_id TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    detail TEXT NOT NULL DEFAULT '',
    user_id INTEGER
);

CREATE INDEX idx_system_events_occurred_at ON system_events(occurred_at);
CREATE INDEX idx_system_events_correlation_id ON system_events(correlation_id);
CREATE INDEX idx_system_events_component ON system_events(component);
CREATE INDEX idx_system_events_severity ON system_events(severity);
CREATE INDEX idx_system_events_event_type ON system_events(event_type);
