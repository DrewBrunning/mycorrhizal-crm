-- T17: drop the composite feed indexes added by the up migration.

DROP INDEX IF EXISTS idx_contacts_feed;
DROP INDEX IF EXISTS idx_notes_feed;
DROP INDEX IF EXISTS idx_activities_feed;
DROP INDEX IF EXISTS idx_life_events_feed;
DROP INDEX IF EXISTS idx_preferences_feed;
DROP INDEX IF EXISTS idx_circles_feed;
DROP INDEX IF EXISTS idx_households_feed;
DROP INDEX IF EXISTS idx_tags_feed;
DROP INDEX IF EXISTS idx_relationship_edges_feed;
DROP INDEX IF EXISTS idx_field_definitions_feed;
