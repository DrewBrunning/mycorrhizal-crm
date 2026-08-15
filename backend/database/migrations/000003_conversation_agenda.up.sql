-- T21 —
-- ConversationAgenda: "things to bring up next time I see them" (§91.11).
-- Contextual memory surfaced on the contact view, explicitly NOT
-- date-scheduled — no due date anywhere, which is what distinguishes it from
-- a Reminder. Resolved by marking it discussed (discussed_at), optionally
-- linking the Activity that covered it (activity_id).
--
-- Soft-delete (deleted_at), per T26: user-authored content. No natural-key
-- unique constraint (a contact may have many agenda items), so a soft-deleted
-- row never blocks re-creation.

CREATE TABLE conversation_agenda (
    id TEXT PRIMARY KEY,
    created_at DATETIME,
    updated_at DATETIME,
    deleted_at DATETIME,
    user_id INTEGER NOT NULL,
    entity_id TEXT NOT NULL,
    content TEXT NOT NULL,
    reference_url TEXT,
    discussed_at DATETIME,
    activity_id INTEGER,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_conversation_agenda_deleted_at ON conversation_agenda(deleted_at);
CREATE INDEX idx_conversation_agenda_entity_id ON conversation_agenda(entity_id);
CREATE INDEX idx_conversation_agenda_feed ON conversation_agenda(user_id, updated_at, id);
CREATE INDEX idx_conversation_agenda_user_id ON conversation_agenda(user_id);
CREATE INDEX idx_conversation_agenda_discussed_at ON conversation_agenda(discussed_at);
CREATE INDEX idx_conversation_agenda_activity_id ON conversation_agenda(activity_id);
