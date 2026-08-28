-- Per-run background-job outcome history (issue #391).
--
-- job_runs is the bounded, first-class record of *how each scheduled job run
-- went* — start/finish, wall-clock duration, success/failure/skipped, the
-- error on the failure path, and an optional count of items the run acted on.
-- It is what turns "the reminder job is red" into "the reminder job has failed
-- 9 times in a row since 03:14, normally takes 1s, last error was <x>", and it
-- is where a birthday reminder that silently fails to send now leaves a trace.
--
-- Deliberately distinct from:
--   * job_executions  — only the lock + last-run bookkeeping (one row per job,
--                       overwritten every run); this is the history it never kept.
--   * system_events   — the operational timeline (issue #424). By design that
--                       stream does NOT get a row per ordinary job completion
--                       ("a per-tick row would swamp the event stream"), only a
--                       job_failed row on a recovered panic. job_runs is the
--                       per-run detail that would have swamped it.
--   * audit_events    — who changed user-authored data (that is user data; this
--                       is operational).
--
-- System-generated, hard-delete (no deleted_at): rows are removed only by the
-- retention purge (JOB_RUN_RETENTION_DAYS, default 30), mirroring
-- system_events' / audit_events' lifecycle (CLAUDE.md backend trap 7).
-- Read-only over the API, admin-only (instance-wide, not user-scoped).
--
-- No high-cardinality fields — no contact IDs, no raw URLs. `error` and
-- `detail` are sanitized + length-capped by the emitter
-- (models.RecordJobRun -> logger.SanitizeLogField). The result / trigger
-- CHECK vocabularies are mirrored by hand in models/job_run.go,
-- frontend/src/api/jobRuns.ts, backend/openapi.yaml, and the Android
-- core/model mirror — a token added in one place without the others is a
-- silent INSERT failure or a missing label (CLAUDE.md frontend trap 4).
CREATE TABLE job_runs (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at      DATETIME NOT NULL,
    job_name        TEXT NOT NULL,
    trigger         TEXT NOT NULL DEFAULT 'scheduled'
                      CHECK (trigger IN ('scheduled', 'initial', 'manual')),
    started_at      DATETIME NOT NULL,
    finished_at     DATETIME NOT NULL,
    duration_ms     INTEGER NOT NULL,
    result          TEXT NOT NULL CHECK (result IN ('success', 'failure', 'skipped')),
    error           TEXT NOT NULL DEFAULT '',
    items_processed INTEGER,
    detail          TEXT NOT NULL DEFAULT '',
    correlation_id  TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_job_runs_started_at ON job_runs(started_at);
CREATE INDEX idx_job_runs_job_name_started_at ON job_runs(job_name, started_at);
CREATE INDEX idx_job_runs_correlation_id ON job_runs(correlation_id);
