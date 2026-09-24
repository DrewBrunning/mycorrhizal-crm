-- Occasions event planning down (docs/adrs/0026-occasions-events.md, issue #1228).
-- Drop the join table before its parent (occasion_events).
DROP TABLE IF EXISTS occasion_event_attendees;
DROP TABLE IF EXISTS occasion_events;
