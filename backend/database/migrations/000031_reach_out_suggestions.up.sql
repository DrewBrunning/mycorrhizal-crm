-- Event-driven "reach out" triggers (issue #177, first cut).
--
-- reach_out_suggestions: one row per detected meaningful change (org/title/
-- address) to a contact, surfaced as a dismissible dashboard suggestion and
-- paired with a one-off Reminder (companion reminder rides the existing
-- email/ntfy/Gotify/push delivery pipeline; reminder_id links back to it so
-- completing/skipping that reminder can flip this row to dismissed).
--
-- System-generated, edge-shaped row (CLAUDE.md backend trap 7): no
-- deleted_at, no natural-key unique constraint to protect — mirrors
-- RelationshipEdge/ExternalActivity's precedent. "Dismiss" is a status
-- update, never a delete.
CREATE TABLE reach_out_suggestions (
    id TEXT PRIMARY KEY,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    user_id INTEGER NOT NULL,
    contact_vcard_uid TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('organization', 'title', 'address')),
    old_value TEXT NOT NULL DEFAULT '',
    new_value TEXT NOT NULL DEFAULT '',
    audit_event_id INTEGER NOT NULL,
    reminder_id INTEGER,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'dismissed'))
);

CREATE INDEX idx_reach_out_suggestions_user_status ON reach_out_suggestions(user_id, status);
CREATE INDEX idx_reach_out_suggestions_contact ON reach_out_suggestions(contact_vcard_uid);
CREATE INDEX idx_reach_out_suggestions_reminder ON reach_out_suggestions(reminder_id);

-- reach_out_cursors: one row per user, the watermark ("last AuditEvent.ID
-- already scanned for reach-out triggers") that lets the detection job pick
-- up exactly where it left off without rescanning history or double-firing.
CREATE TABLE reach_out_cursors (
    user_id INTEGER PRIMARY KEY,
    last_audit_event_id INTEGER NOT NULL DEFAULT 0,
    updated_at DATETIME NOT NULL
);
