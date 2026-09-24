-- Occasions event planning (docs/adrs/0026-occasions-events.md, issue #1228):
-- a one-off event the user is hosting, plus its invitee/RSVP ledger. The
-- design pass settled that this is a NEW first-class entity, not a
-- specialization of calendar_event_links (a machine-owned iCal import mapping)
-- and not an extension of occasion_obligations (a standing annual rule).
--
-- occasion_events: user-authored content -> soft delete (deleted_at), same
-- shape as occasion_obligations. starts_at/ends_at are RFC 3339 instants
-- (ADR 0015 category 1); ends_at is optional. No revision/etag: no sync
-- surface, no conditional-write requirement (ADR 0026 part 5).
CREATE TABLE occasion_events (
    id TEXT PRIMARY KEY,
    created_at DATETIME,
    updated_at DATETIME,
    deleted_at DATETIME,
    user_id INTEGER NOT NULL,
    title TEXT NOT NULL,
    starts_at DATETIME NOT NULL,
    ends_at DATETIME,
    location TEXT,
    sensitivity TEXT NOT NULL DEFAULT 'normal',
    notes TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX idx_occasion_events_deleted_at ON occasion_events(deleted_at);
CREATE INDEX idx_occasion_events_user_id ON occasion_events(user_id);
CREATE INDEX idx_occasion_events_feed ON occasion_events(user_id, updated_at, id);
CREATE INDEX idx_occasion_events_starts_at ON occasion_events(starts_at);
CREATE INDEX idx_occasion_events_sensitivity ON occasion_events(sensitivity);

-- occasion_event_attendees: the invitee/RSVP join row. Hard delete (join-shaped,
-- natural key (event_id, entity_id) -- ADR 0004/CLAUDE.md trap 7); a soft-deleted
-- row would block re-inviting the same contact. entity_id is a
-- Contact.VCardUID (the graph invariant), NOT a contacts.id FK -- the same
-- "re-pull rather than track deaths" reasoning as circle_members. rsvp is a
-- manually-recorded status, not a delivered invitation (ADR 0026 part 2).
CREATE TABLE occasion_event_attendees (
    id TEXT PRIMARY KEY,
    created_at DATETIME,
    updated_at DATETIME,
    user_id INTEGER NOT NULL,
    event_id TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    rsvp TEXT NOT NULL DEFAULT 'pending',
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (event_id) REFERENCES occasion_events(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX idx_occasion_event_attendees_event_entity ON occasion_event_attendees(event_id, entity_id);
CREATE INDEX idx_occasion_event_attendees_user_id ON occasion_event_attendees(user_id);
CREATE INDEX idx_occasion_event_attendees_event_id ON occasion_event_attendees(event_id);
CREATE INDEX idx_occasion_event_attendees_entity_id ON occasion_event_attendees(entity_id);
