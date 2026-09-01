-- Rollback of 000048 (issue #526). last_outcome is derived diagnostic state
-- rebuilt by the job lock on the next run of each job, so dropping it loses
-- nothing durable.
ALTER TABLE job_executions DROP COLUMN last_outcome;
