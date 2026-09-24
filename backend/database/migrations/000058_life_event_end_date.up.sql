-- Temporal periods (issue #354, docs/adrs/0023-temporal-periods.md).
--
-- LifeEvent.Date is the start/anchor of an event; end_date makes it a span
-- ("worked at Acme 2019-2024"). It is a PartialDate serialized as JSON text,
-- matching the existing `date` column's storage convention (a calendar date
-- with one or more components deliberately unknown; no time, no zone).
--
-- NULL is the correct value for every existing row: an event with no end is a
-- point in time, which is exactly what those rows are. No backfill.
ALTER TABLE life_events ADD COLUMN end_date TEXT;
