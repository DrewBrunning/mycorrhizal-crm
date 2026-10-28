-- Circle description and colour (issue #1995, Part 1).
--
-- circles.description: free text, trimmed, at most 500 characters (enforced in
-- the API, not here). circles.color: a palette token (models.CircleColors), ''
-- meaning "no colour". Both are internal-only: circles have no standards
-- projection, so neither column reaches the neutral Card, CardDAV or CalDAV.
--
-- No backfill needed: NOT NULL DEFAULT '' gives every existing circle the same
-- name-only, uncoloured appearance it has today.
ALTER TABLE circles ADD COLUMN description TEXT NOT NULL DEFAULT '';
ALTER TABLE circles ADD COLUMN color TEXT NOT NULL DEFAULT '';
