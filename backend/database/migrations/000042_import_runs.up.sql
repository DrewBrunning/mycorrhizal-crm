-- Per-import outcome history (issue #651).
--
-- import_runs is the lightweight, first-class record of *what each confirmed
-- contact import did* — one row per completed import, holding the source
-- format and the same five counts the HTTP response already returns
-- (total processed / created / updated / skipped / error count). It turns
-- "did last week's import drop anything?" from unanswerable into a row on the
-- Data settings page. This is the "import history/status" gap carved out of
-- the former #388 epic; async import is out of scope.
--
-- Immutable operational bookkeeping, hard-delete only (no deleted_at), the
-- same lifecycle shape as job_runs / alert_states / operational_check_results
-- (CLAUDE.md backend trap #7). Unlike those, it *is* user-scoped: a row
-- belongs to the user who ran the import and is swept by DeleteUser's manual
-- cascade (backend trap #6) — the canonical checklist is DeleteContact in
-- contact_controller.go.
--
-- No high-cardinality fields: counts, a timestamp, and a small fixed-vocabulary
-- format token. No contact content, no names, no PII. The `format` CHECK
-- vocabulary is mirrored by hand in models/import_run.go,
-- frontend/src/api/import.ts, and backend/openapi.yaml — a token added in one
-- place without the others is a silent INSERT failure or a missing label
-- (CLAUDE.md frontend trap #4).
CREATE TABLE import_runs (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL,
    format          TEXT NOT NULL CHECK (format IN ('csv', 'vcf', 'jscontact', 'records')),
    total_processed INTEGER NOT NULL DEFAULT 0,
    created         INTEGER NOT NULL DEFAULT 0,
    updated         INTEGER NOT NULL DEFAULT 0,
    skipped         INTEGER NOT NULL DEFAULT 0,
    error_count     INTEGER NOT NULL DEFAULT 0,
    created_at      DATETIME NOT NULL
);

CREATE INDEX idx_import_runs_user ON import_runs(user_id, created_at);
