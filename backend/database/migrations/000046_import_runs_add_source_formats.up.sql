-- Widens the import_runs.format CHECK (issue #549, and #550 in advance) to
-- add the source-import formats 'monica' and 'meerkat', alongside the
-- existing file formats. The Monica import assistant records one import_runs
-- row per confirmed import so it shows up in the Data settings history table
-- like every other import; without a token for it the INSERT fails silently
-- (models.RecordImportRun is best-effort).
--
-- SQLite cannot ALTER a CHECK constraint, so this rebuilds the table exactly
-- like 000035 rebuilt audit_events. import_runs has no triggers and no
-- foreign keys, so the rebuild is a plain create/copy/rename. All existing
-- rows are preserved verbatim. The `format` vocabulary is mirrored by hand
-- in models/import_run.go, frontend/src/api/import.ts, and
-- backend/openapi.yaml (CLAUDE.md frontend trap #4).

CREATE TABLE import_runs_new (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL,
    format          TEXT NOT NULL CHECK (format IN ('csv', 'vcf', 'jscontact', 'records', 'monica', 'meerkat')),
    total_processed INTEGER NOT NULL DEFAULT 0,
    created         INTEGER NOT NULL DEFAULT 0,
    updated         INTEGER NOT NULL DEFAULT 0,
    skipped         INTEGER NOT NULL DEFAULT 0,
    error_count     INTEGER NOT NULL DEFAULT 0,
    created_at      DATETIME NOT NULL
);

INSERT INTO import_runs_new (id, user_id, format, total_processed, created, updated, skipped, error_count, created_at)
SELECT id, user_id, format, total_processed, created, updated, skipped, error_count, created_at FROM import_runs;

DROP TABLE import_runs;
ALTER TABLE import_runs_new RENAME TO import_runs;

CREATE INDEX idx_import_runs_user ON import_runs(user_id, created_at);
