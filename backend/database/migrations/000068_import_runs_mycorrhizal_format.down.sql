-- Rollback of 000068: narrows import_runs.format back to the pre-#1260
-- vocabulary. Rows recorded under the now-invalid `mycorrhizal` token would
-- violate the CHECK, so they are removed first. import_runs is immutable
-- operational bookkeeping (counts and a timestamp, no user content), so
-- dropping those rows is safe — the account bundle itself is unaffected.
DELETE FROM import_runs WHERE format = 'mycorrhizal';

CREATE TABLE import_runs_old (
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

INSERT INTO import_runs_old (id, user_id, format, total_processed, created, updated, skipped, error_count, created_at)
SELECT id, user_id, format, total_processed, created, updated, skipped, error_count, created_at FROM import_runs;

DROP TABLE import_runs;
ALTER TABLE import_runs_old RENAME TO import_runs;

CREATE INDEX idx_import_runs_user ON import_runs(user_id, created_at);
