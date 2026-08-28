-- Per-run background-job outcome history (issue #391) — rollback. job_runs is
-- system-generated diagnostic data with a bounded retention window; dropping it
-- loses only operational history, no user data.
DROP TABLE job_runs;
