-- Occasions reminder integration (docs/adrs/0024-occasions.md, issue #387,
-- ticket #1223): reminders.occasion_obligation_id mirrors reminders.life_event_id
-- exactly — a nullable soft reference (no FK) so syncOccasionObligationReminder
-- can find and hard-delete/regenerate the one materialized reminder for a given
-- obligation, the same way syncLifeEventReminder does for life events.

ALTER TABLE reminders ADD COLUMN occasion_obligation_id TEXT;
CREATE INDEX idx_reminders_occasion_obligation_id ON reminders(occasion_obligation_id);
