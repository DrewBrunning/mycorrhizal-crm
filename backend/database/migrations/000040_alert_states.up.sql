-- alert_states — one row per alert condition, holding whether that condition is
-- currently raised and since when (issue #428). The scheduled alert evaluator
-- (services.EvaluateAlerts) compares each condition's freshly-computed state
-- against the row here and dispatches a notification ONLY on a transition, so a
-- condition that keeps failing produces one alert, not one per evaluation.
--
-- Hard state, one row per condition_key (upserted) — operational bookkeeping
-- like job_executions and operational_check_results (migration 000037), not
-- user-authored content, so no deleted_at (CLAUDE.md backend trap #7). Holds no
-- user data: a condition key, an ok/alerting state, a timestamp, a short
-- sanitized detail string (e.g. "disk usage 92% of /var/lib/mycorrhizal"), and
-- the consecutive-failure count captured when the alert was raised (so the
-- recovery notification can say "recovered after N failures").

CREATE TABLE alert_states (
    condition_key    TEXT PRIMARY KEY,
    state            TEXT NOT NULL CHECK (state IN ('ok', 'alerting')),
    since            DATETIME NOT NULL,
    detail           TEXT NOT NULL DEFAULT '',
    failure_count    INTEGER NOT NULL DEFAULT 0,
    last_notified_at DATETIME,
    created_at       DATETIME,
    updated_at       DATETIME
);
