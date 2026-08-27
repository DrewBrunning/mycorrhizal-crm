-- Reverses 000040. The table holds only regenerable alert bookkeeping (no user
-- data): the next run of the scheduled alert evaluator rebuilds every row from
-- the current subsystem-health state. Dropping it loses only the "since when"
-- timestamps for any currently-open incident.

DROP TABLE IF EXISTS alert_states;
