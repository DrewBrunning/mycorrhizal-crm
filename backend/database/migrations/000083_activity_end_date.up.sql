-- Optional end date for multi-day activities (issue #2006).
--
-- activities.end_date: nullable DATETIME, same calendar-day shape as
-- activities.date (ADR 0015 category 2: the picked day at 00:00:00Z, read back
-- as its UTC date). NULL means a single-day activity.
--
-- No backfill: every existing activity stays single-day exactly as today.
ALTER TABLE activities ADD COLUMN end_date DATETIME;
