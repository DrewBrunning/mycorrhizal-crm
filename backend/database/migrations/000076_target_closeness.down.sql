-- Rollback of 000076 (issue #1796). target_behind rows are system-generated
-- and re-derived daily by the reach_out_detection job, so deleting them on
-- rollback is acceptable (they would violate the narrowed CHECK). Every
-- organization/title/address row is preserved. Dropping target_closeness
-- loses the user-set targets (nothing else stores them).
DELETE FROM reach_out_suggestions WHERE kind = 'target_behind';

CREATE TABLE reach_out_suggestions_old (
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

INSERT INTO reach_out_suggestions_old (id, created_at, updated_at, user_id, contact_vcard_uid, kind, old_value, new_value, audit_event_id, reminder_id, status)
SELECT id, created_at, updated_at, user_id, contact_vcard_uid, kind, old_value, new_value, audit_event_id, reminder_id, status FROM reach_out_suggestions;

DROP TABLE reach_out_suggestions;
ALTER TABLE reach_out_suggestions_old RENAME TO reach_out_suggestions;

CREATE INDEX idx_reach_out_suggestions_user_status ON reach_out_suggestions(user_id, status);
CREATE INDEX idx_reach_out_suggestions_contact ON reach_out_suggestions(contact_vcard_uid);
CREATE INDEX idx_reach_out_suggestions_reminder ON reach_out_suggestions(reminder_id);

ALTER TABLE contacts DROP COLUMN target_closeness;
