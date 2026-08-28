-- Per-import outcome history (issue #651) — rollback. import_runs holds only
-- row counts + timestamps for completed imports; dropping it loses that
-- operational history and nothing else (no contact data, no PII).
DROP TABLE import_runs;
