-- Per-contact field visibility and hide-empty mode (issue #1997).
--
-- contact_field_overrides holds a user's cosmetic, per-contact overrides of
-- which extended contact fields the UI shows. It sits outside the neutral Card
-- by construction: a separate owner-scoped table keyed by (user_id,
-- contact_vcard_uid, field_key), so it can never reach the vCard/JSContact
-- exporters, CardDAV/CalDAV, shares or MCP, and writing it does not touch the
-- contact row or its revision.
--
-- state is tri-state in the API (inherit / show / hide); "inherit" is the
-- ABSENCE of a row, so only the two explicit states are stored. The reserved
-- field_key '__hide_empty' carries the per-contact hide-empty override in the
-- same table (state 'hide' = hide empty fields, 'show' = show them); every
-- other key is a toggleable contact field, allowlisted in the application.
--
-- Hard delete (join-shaped preference row, trap #7): deleting a contact sweeps
-- its rows in the cascade registry and the trash snapshot restores them.
CREATE TABLE contact_field_overrides (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    user_id INTEGER NOT NULL,
    contact_vcard_uid TEXT NOT NULL,
    field_key TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('show', 'hide')),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX idx_contact_field_override_key
    ON contact_field_overrides(user_id, contact_vcard_uid, field_key);
CREATE INDEX idx_contact_field_overrides_user_id
    ON contact_field_overrides(user_id);

-- users.hide_empty_fields: the account-wide default for hide-empty mode.
-- NOT NULL DEFAULT 0 keeps every existing user's display exactly as it is.
ALTER TABLE users ADD COLUMN hide_empty_fields BOOLEAN NOT NULL DEFAULT 0;
