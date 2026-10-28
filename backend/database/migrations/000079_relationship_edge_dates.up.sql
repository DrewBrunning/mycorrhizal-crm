-- First-class relationship dates (issue #2001).
--
-- start_date / end_date are PartialDate JSON text, the same storage convention
-- as life_events.date / life_events.end_date (migration 000058, ADR 0025): a
-- calendar date with components deliberately unknown, no time, no zone. NULL
-- means "not recorded"; an edge with no end_date is open-ended.
ALTER TABLE relationship_edges ADD COLUMN start_date TEXT;
ALTER TABLE relationship_edges ADD COLUMN end_date TEXT;

-- Backfill from the free-form metadata keys the graph has always carried
-- ("since" / "until"). Only a value that is unambiguously a calendar date
-- (YYYY, YYYY-MM or YYYY-MM-DD with a real month/day range) is promoted; any
-- other value stays readable in metadata and the typed column stays NULL.
-- metadata is NEVER modified here, so nothing is lost and the down migration
-- has nothing to restore.
UPDATE relationship_edges
SET start_date = CASE
    WHEN json_extract(metadata, '$.since') GLOB '[0-9][0-9][0-9][0-9]'
        THEN json_object('year', CAST(json_extract(metadata, '$.since') AS INTEGER))
    WHEN json_extract(metadata, '$.since') GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]'
         AND CAST(substr(json_extract(metadata, '$.since'), 6, 2) AS INTEGER) BETWEEN 1 AND 12
        THEN json_object('year', CAST(substr(json_extract(metadata, '$.since'), 1, 4) AS INTEGER),
                         'month', CAST(substr(json_extract(metadata, '$.since'), 6, 2) AS INTEGER))
    WHEN json_extract(metadata, '$.since') GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'
         AND CAST(substr(json_extract(metadata, '$.since'), 6, 2) AS INTEGER) BETWEEN 1 AND 12
         AND CAST(substr(json_extract(metadata, '$.since'), 9, 2) AS INTEGER) BETWEEN 1 AND 31
        THEN json_object('year', CAST(substr(json_extract(metadata, '$.since'), 1, 4) AS INTEGER),
                         'month', CAST(substr(json_extract(metadata, '$.since'), 6, 2) AS INTEGER),
                         'day', CAST(substr(json_extract(metadata, '$.since'), 9, 2) AS INTEGER))
    END,
    end_date = CASE
    WHEN json_extract(metadata, '$.until') GLOB '[0-9][0-9][0-9][0-9]'
        THEN json_object('year', CAST(json_extract(metadata, '$.until') AS INTEGER))
    WHEN json_extract(metadata, '$.until') GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]'
         AND CAST(substr(json_extract(metadata, '$.until'), 6, 2) AS INTEGER) BETWEEN 1 AND 12
        THEN json_object('year', CAST(substr(json_extract(metadata, '$.until'), 1, 4) AS INTEGER),
                         'month', CAST(substr(json_extract(metadata, '$.until'), 6, 2) AS INTEGER))
    WHEN json_extract(metadata, '$.until') GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'
         AND CAST(substr(json_extract(metadata, '$.until'), 6, 2) AS INTEGER) BETWEEN 1 AND 12
         AND CAST(substr(json_extract(metadata, '$.until'), 9, 2) AS INTEGER) BETWEEN 1 AND 31
        THEN json_object('year', CAST(substr(json_extract(metadata, '$.until'), 1, 4) AS INTEGER),
                         'month', CAST(substr(json_extract(metadata, '$.until'), 6, 2) AS INTEGER),
                         'day', CAST(substr(json_extract(metadata, '$.until'), 9, 2) AS INTEGER))
    END
WHERE metadata IS NOT NULL AND json_valid(metadata) AND json_type(metadata) = 'object';

-- A backfilled pair that is reversed (until before since) would violate the
-- end >= start rule the API now enforces. Keep both facts but drop the
-- promoted end date; the original value is still in metadata.until.
UPDATE relationship_edges
SET end_date = NULL
WHERE start_date IS NOT NULL AND end_date IS NOT NULL
  AND (
    json_extract(end_date, '$.year') < json_extract(start_date, '$.year')
    OR (json_extract(end_date, '$.year') = json_extract(start_date, '$.year')
        AND json_extract(end_date, '$.month') IS NOT NULL AND json_extract(start_date, '$.month') IS NOT NULL
        AND json_extract(end_date, '$.month') < json_extract(start_date, '$.month'))
    OR (json_extract(end_date, '$.year') = json_extract(start_date, '$.year')
        AND json_extract(end_date, '$.month') = json_extract(start_date, '$.month')
        AND json_extract(end_date, '$.day') IS NOT NULL AND json_extract(start_date, '$.day') IS NOT NULL
        AND json_extract(end_date, '$.day') < json_extract(start_date, '$.day'))
  );
