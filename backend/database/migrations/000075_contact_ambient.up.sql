-- Ambient contacts (issue #1733, ADR 0023 amendment).
--
-- A single additive boolean on contacts: the user's explicit "I see this person
-- all the time" flag. An ambient contact counts as in regular contact for the
-- relationship health score and for cadence overdue without logging activities.
-- (Co-residence is the other source of "ambient" and is derived from the
-- household tables at read time, so nothing is stored for it.)
--
-- No backfill needed: DEFAULT 0 makes every existing row non-ambient, which is
-- exactly today's behaviour. No index: reads are always over one user's rows and
-- the flag is read as part of the row, never filtered on.

ALTER TABLE contacts ADD COLUMN ambient BOOLEAN NOT NULL DEFAULT 0;
