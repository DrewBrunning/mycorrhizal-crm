-- Per-contact target closeness (issue #1737 / #1796, ADR 0023 amendment
-- 2026-10-16).
--
-- 1. contacts.target_closeness: the user's relationship goal for a contact,
--    NULL = no target. No default and no backfill: every existing row reads
--    NULL, which is exactly today's behaviour.
-- 2. reach_out_suggestions.kind CHECK widened to add 'target_behind'. SQLite
--    cannot ALTER a CHECK, so the table is rebuilt exactly like 000068 did:
--    every row is copied verbatim and the three indexes are recreated.
--    The kind vocabulary is mirrored by hand in models/reach_out_suggestion.go,
--    frontend/src/api/reachOutSuggestions.ts and backend/openapi.yaml.

ALTER TABLE contacts ADD COLUMN target_closeness TEXT CHECK (target_closeness IS NULL OR target_closeness IN ('immediate','close','casual','distant'));

CREATE TABLE reach_out_suggestions_new (
    id TEXT PRIMARY KEY,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    user_id INTEGER NOT NULL,
    contact_vcard_uid TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('organization', 'title', 'address', 'target_behind')),
    old_value TEXT NOT NULL DEFAULT '',
    new_value TEXT NOT NULL DEFAULT '',
    audit_event_id INTEGER NOT NULL,
    reminder_id INTEGER,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'dismissed'))
);

INSERT INTO reach_out_suggestions_new (id, created_at, updated_at, user_id, contact_vcard_uid, kind, old_value, new_value, audit_event_id, reminder_id, status)
SELECT id, created_at, updated_at, user_id, contact_vcard_uid, kind, old_value, new_value, audit_event_id, reminder_id, status FROM reach_out_suggestions;

DROP TABLE reach_out_suggestions;
ALTER TABLE reach_out_suggestions_new RENAME TO reach_out_suggestions;

CREATE INDEX idx_reach_out_suggestions_user_status ON reach_out_suggestions(user_id, status);
CREATE INDEX idx_reach_out_suggestions_contact ON reach_out_suggestions(contact_vcard_uid);
CREATE INDEX idx_reach_out_suggestions_reminder ON reach_out_suggestions(reminder_id);
