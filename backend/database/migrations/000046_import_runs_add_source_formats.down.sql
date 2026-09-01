-- Reverts 000046: narrows the import_runs.format CHECK back to the file
-- formats only. This is destructive to history if any 'monica' or 'meerkat'
-- row exists — the INSERT below drops those rows (a CHECK violation would
-- otherwise abort the whole migration). That is acceptable for a down
-- migration of an operational bookkeeping table (import_runs is immutable,
-- non-PII counts, not user content); it never touches imported contacts.

CREATE TABLE import_runs_new (
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

INSERT INTO import_runs_new (id, user_id, format, total_processed, created, updated, skipped, error_count, created_at)
SELECT id, user_id, format, total_processed, created, updated, skipped, error_count, created_at FROM import_runs
WHERE format IN ('csv', 'vcf', 'jscontact', 'records');

DROP TABLE import_runs;
ALTER TABLE import_runs_new RENAME TO import_runs;

CREATE INDEX idx_import_runs_user ON import_runs(user_id, created_at);
