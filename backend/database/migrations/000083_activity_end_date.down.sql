-- Optional end date for multi-day activities (issue #2006) -- rollback. Drops
-- the column; any end date set since the upgrade is lost and those activities
-- revert to their single start day.
ALTER TABLE activities DROP COLUMN end_date;
