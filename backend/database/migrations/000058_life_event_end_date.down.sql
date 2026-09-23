-- Rollback of 000058 (issue #354). end_date is user-authored content, but the
-- down migration is only ever run deliberately, and the option that holds real
-- data is the up direction. Dropping the column loses any recorded end date;
-- date (the start/anchor) remains.
ALTER TABLE life_events DROP COLUMN end_date;
