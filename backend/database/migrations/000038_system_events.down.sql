-- Structured operational-event model + system event timeline (issue #424) —
-- rollback. system_events is system-generated diagnostic data with a bounded
-- retention window; dropping it loses only operational history, no user data.
DROP TABLE system_events;
