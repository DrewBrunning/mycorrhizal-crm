-- T20b —
-- Gift: "what did I give them last year?" (§91.11) against a contact, covering
-- the three states that make the feature useful — an idea captured
-- opportunistically, the offered/given gift with its date, and a received gift
-- (reciprocity / say-thanks tracking). Plus occasion, an optional value with an
-- explicit currency (value_cents + ISO-4217 currency code), and an optional
-- link to the LifeEvent (life_event_id) or Activity (activity_id) it relates
-- to.
--
-- Soft-delete (deleted_at), per T26: user-authored content. No natural-key
-- unique constraint (a contact may have many gifts), so a soft-deleted row
-- never blocks re-creation.

CREATE TABLE gifts (
    id TEXT PRIMARY KEY,
    created_at DATETIME,
    updated_at DATETIME,
    deleted_at DATETIME,
    user_id INTEGER NOT NULL,
    entity_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'idea',
    occasion TEXT,
    description TEXT NOT NULL,
    date DATETIME,
    value_cents INTEGER,
    currency TEXT,
    life_event_id TEXT,
    activity_id INTEGER,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_gifts_deleted_at ON gifts(deleted_at);
CREATE INDEX idx_gifts_entity_id ON gifts(entity_id);
CREATE INDEX idx_gifts_feed ON gifts(user_id, updated_at, id);
CREATE INDEX idx_gifts_user_id ON gifts(user_id);
CREATE INDEX idx_gifts_status ON gifts(status);
CREATE INDEX idx_gifts_date ON gifts(date);
CREATE INDEX idx_gifts_life_event_id ON gifts(life_event_id);
CREATE INDEX idx_gifts_activity_id ON gifts(activity_id);
