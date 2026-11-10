-- Issue #2463 (part of #2210): persist the per-row issues of a confirmed import
-- so the import-history API can return an itemized report.
--
-- Two additive columns on import_runs rather than a child table: the issues are
-- written once with the run, immutable, read as a unit and capped at 1000 per
-- run (models.MaxImportRunIssues), so a table would add a cascade entry and a
-- join for no query benefit.
--   issues       JSON array of {row,name,status,reason} (email and phone are
--                never stored, see docs/security/pii-inventory.md)
--   issue_count  number of stored issues, so the list endpoint can say whether
--                a report exists without loading the JSON
-- Existing rows get '[]' / 0: their per-row detail was never stored and is not
-- backfilled. No existing value changes.
ALTER TABLE import_runs ADD COLUMN issues TEXT NOT NULL DEFAULT '[]';
ALTER TABLE import_runs ADD COLUMN issue_count INTEGER NOT NULL DEFAULT 0;
