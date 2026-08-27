-- operational_check_results — one row per named operational self-check, holding
-- that check's most recent outcome (issue #421). The deep /health endpoint
-- reads it so an operator can see the last DB-integrity-check and restore-drill
-- results (actual pass/fail, not just "did it run"); before this table those
-- outcomes only ever reached the logs and a failure webhook.
--
-- Hard state, one row per check_name (upserted) — operational bookkeeping like
-- job_executions, not user-authored content, so no deleted_at (CLAUDE.md
-- backend trap #7). Holds no user data: a check name, an ok/failed/error
-- status, and a short detail string (e.g. a restore-drill row-count mismatch
-- like "contacts: live=10 restored=9" or an integrity_check problem line).

CREATE TABLE operational_check_results (
    check_name TEXT PRIMARY KEY,
    status     TEXT NOT NULL CHECK (status IN ('ok', 'failed', 'error')),
    detail     TEXT NOT NULL DEFAULT '',
    checked_at DATETIME NOT NULL,
    created_at DATETIME,
    updated_at DATETIME
);
