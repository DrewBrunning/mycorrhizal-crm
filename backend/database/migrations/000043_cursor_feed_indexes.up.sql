-- T17: composite (user_id, updated_at, id) indexes backing cursor pagination
-- and the ?since= change feed on every paginated list endpoint.
--
-- The cursor scheme orders by (updated_at, id) — a bare updated_at is not
-- unique, so id breaks ties at page boundaries. Without a composite index the
-- row-value predicate `(updated_at, id) > (?, ?)` (and the ORDER BY that goes
-- with it) degrades to a table scan per page. All of these tables already
-- carry a user_id column; leading the index with it keeps the whole query
-- (ownership scope + cursor range) a single index walk.

CREATE INDEX IF NOT EXISTS idx_contacts_feed ON contacts(user_id, updated_at, id);
CREATE INDEX IF NOT EXISTS idx_notes_feed ON notes(user_id, updated_at, id);
CREATE INDEX IF NOT EXISTS idx_activities_feed ON activities(user_id, updated_at, id);
CREATE INDEX IF NOT EXISTS idx_life_events_feed ON life_events(user_id, updated_at, id);
CREATE INDEX IF NOT EXISTS idx_preferences_feed ON preferences(user_id, updated_at, id);
CREATE INDEX IF NOT EXISTS idx_circles_feed ON circles(user_id, updated_at, id);
CREATE INDEX IF NOT EXISTS idx_households_feed ON households(user_id, updated_at, id);
CREATE INDEX IF NOT EXISTS idx_tags_feed ON tags(user_id, updated_at, id);
CREATE INDEX IF NOT EXISTS idx_relationship_edges_feed ON relationship_edges(user_id, updated_at, id);
CREATE INDEX IF NOT EXISTS idx_field_definitions_feed ON field_definitions(user_id, updated_at, id);
