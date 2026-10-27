-- Circle description and colour (issue #1995) -- rollback. Drops both columns;
-- any description or colour set since the upgrade is lost (nothing else stores
-- it) and circles revert to name-only.
ALTER TABLE circles DROP COLUMN color;
ALTER TABLE circles DROP COLUMN description;
