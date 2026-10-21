-- User time-format preference (issue #1830).
--
-- users.time_format: 'auto' | '12h' | '24h'. 'auto' defers to the platform
-- (Android: the device 12/24-hour setting; web: the locale default); '12h' and
-- '24h' override it. Modelled on users.date_format.
--
-- No backfill needed: NOT NULL DEFAULT 'auto' gives every existing user row
-- 'auto', which is exactly today's behaviour (times follow the locale/device).
ALTER TABLE users ADD COLUMN time_format TEXT NOT NULL DEFAULT 'auto';
