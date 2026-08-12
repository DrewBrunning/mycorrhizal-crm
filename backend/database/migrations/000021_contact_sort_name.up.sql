-- T73 (docs/fork-plan/tickets/117-T73-contacts-list-sort-control.md) — the
-- contacts list can only be sorted by most-recently-edited (T17's
-- compile-time-fixed (updated_at, id) cursor order). This migration adds the
-- denormalized name-sort key the new ?sort=name control orders by.
--
-- Why a denormalized column and not `ORDER BY lastname, firstname`:
--   * `firstname`/`lastname` are declared COLLATE NOCASE (models/contact.go),
--     and a cursor's row-value predicate must compare under exactly the same
--     collation as its ORDER BY or pagination silently skips and repeats rows
--     at page boundaries. Pre-lowercasing the key removes collation from the
--     equation entirely.
--   * a personal CRM has many first-name-only contacts; sorting by a bare
--     `lastname` would clump every one of them at one end.
--
-- Rule (mirrors models.DeriveSortName, models/contact.go):
--   sort_name = lower(trim(lastname))  when non-empty
--             = lower(trim(firstname)) otherwise
--
-- Never empty for a valid contact (Firstname is required), pre-lowercased,
-- and NOT part of the API surface — derived data, rebuildable at any time,
-- exactly like addresses_flat (T38) and phones_normalized (T69). Backfilled
-- here for rows that predate the migration and will never be re-saved.
-- Not added to contacts_fts: sort_name is for ordering, not search.
--
-- The index mirrors the (user_id, updated_at, id) feed-index pattern
-- (idx_contacts_feed): a name-sorted page is ordered by
-- (sort_name, id) under a user scope, so it needs the same composite shape
-- or the cursor query degrades to a scan.

ALTER TABLE contacts ADD COLUMN sort_name TEXT NOT NULL DEFAULT '';

-- COALESCE guards the two nullable-input traps so the backfill can never
-- write NULL into the NOT NULL column and never drift from Go's
-- DeriveSortName (which scans a NULL lastname as ""): length(trim(NULL))
-- would evaluate to NULL and fall through to ELSE, and lower(trim(NULL))
-- would then be NULL — a constraint violation that would abort the whole
-- migration.
UPDATE contacts
SET sort_name = CASE
    WHEN length(trim(COALESCE(lastname, ''))) > 0 THEN lower(trim(lastname))
    ELSE lower(trim(COALESCE(firstname, '')))
END;

CREATE INDEX idx_contacts_sort_name ON contacts(user_id, sort_name, id);
