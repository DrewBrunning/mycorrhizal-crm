-- Occasions (docs/adrs/0024-occasions.md, issue #387, ticket #1222): a standing,
-- recurring obligation toward a contact — "this contact is on my holiday
-- card list every year", "get them a birthday gift, ordered two weeks
-- ahead". The missing "standing rule" layer above LifeEvent/Gift/Preference.
--
-- Soft-delete (deleted_at): user-authored content, same shape as
-- LifeEvent/Preference/Gift. No natural-key unique constraint (a contact may
-- have several obligations of the same kind), so a soft-deleted row never
-- blocks re-creation. No revision/etag column: this entity has no
-- CardDAV/CalDAV sync surface and no conditional-write requirement, unlike
-- LifeEvent/Reminder.

CREATE TABLE occasion_obligations (
    id TEXT PRIMARY KEY,
    created_at DATETIME,
    updated_at DATETIME,
    deleted_at DATETIME,
    user_id INTEGER NOT NULL,
    entity_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    label TEXT NOT NULL,
    anchor_month INTEGER,
    anchor_day INTEGER,
    linked_life_event_id TEXT,
    lead_time_days INTEGER NOT NULL DEFAULT 0,
    active BOOLEAN NOT NULL DEFAULT 1,
    sensitivity TEXT NOT NULL DEFAULT 'normal',
    notes TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX idx_occasion_obligations_deleted_at ON occasion_obligations(deleted_at);
CREATE INDEX idx_occasion_obligations_entity_id ON occasion_obligations(entity_id);
CREATE INDEX idx_occasion_obligations_user_id ON occasion_obligations(user_id);
CREATE INDEX idx_occasion_obligations_feed ON occasion_obligations(user_id, updated_at, id);
CREATE INDEX idx_occasion_obligations_sensitivity ON occasion_obligations(sensitivity);
CREATE INDEX idx_occasion_obligations_linked_life_event_id ON occasion_obligations(linked_life_event_id);
CREATE INDEX idx_occasion_obligations_kind ON occasion_obligations(kind);
CREATE INDEX idx_occasion_obligations_active ON occasion_obligations(active);
