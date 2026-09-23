-- Infer-and-suggest: permanent resolution memory for inferred life-event
-- candidates (issue #354 follow-up, docs/adrs/0023-temporal-periods.md).
--
-- When a dated field period (an address/employer start) looks like it might be
-- a life event, the app *suggests* a candidate event. It is not stored as a
-- LifeEvent and never appears on the timeline unless the user accepts it. This
-- table records the decision so a resolved candidate is not offered again.
--
-- The identity is the natural key (user_id, entity_id, source_kind,
-- source_entry_id, event_type): the same period always produces the same
-- candidate, so once resolved it stays resolved. Hard-delete per T26 (a
-- join-shaped row whose identity IS its natural key), no deleted_at column and
-- no partial index.

CREATE TABLE life_event_suggestion_resolutions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    user_id INTEGER NOT NULL,
    entity_id TEXT NOT NULL,
    source_kind TEXT NOT NULL,
    source_entry_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    resolution TEXT NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX idx_life_event_suggestion_resolution
    ON life_event_suggestion_resolutions(user_id, entity_id, source_kind, source_entry_id, event_type);
CREATE INDEX idx_life_event_suggestion_resolutions_entity_id
    ON life_event_suggestion_resolutions(entity_id);
CREATE INDEX idx_life_event_suggestion_resolutions_user_id
    ON life_event_suggestion_resolutions(user_id);
