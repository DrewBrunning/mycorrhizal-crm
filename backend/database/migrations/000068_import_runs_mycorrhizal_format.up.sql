-- Widens the import_runs.format CHECK to add the `mycorrhizal` account-bundle
-- source (issue #1260, ADR 0028 Decision 3). SQLite cannot ALTER a CHECK
-- constraint, so this rebuilds the table exactly like 000046 did; all existing
-- rows are preserved verbatim. The vocabulary is mirrored by hand in
-- models/import_run.go, frontend/src/api/import.ts, and backend/openapi.yaml
-- (CLAUDE.md frontend trap #4).

CREATE TABLE import_runs_new (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL,
    format          TEXT NOT NULL CHECK (format IN ('csv', 'vcf', 'jscontact', 'records', 'monica', 'meerkat', 'mycorrhizal')),
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
