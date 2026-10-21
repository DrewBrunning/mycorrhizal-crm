-- User time-format preference (issue #1830) -- rollback. Drops the column; any
-- explicit '12h'/'24h' choice made since the upgrade is lost (nothing else
-- stores it) and times revert to following the locale/device.
ALTER TABLE users DROP COLUMN time_format;
