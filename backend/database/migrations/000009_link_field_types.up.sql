-- T34 — LinkFieldType,
-- the user-configurable registry of messaging/social link types (name,
-- URI-template protocol with a {value} placeholder, category, icon,
-- position). This is a deliberate exception to the usual hardcoded-enum
-- convention (see the ticket's "Decisions made" section) — closer to
-- Monica's "Contact field types" than a backend `oneof` mirror.
--
-- Soft-delete (deleted_at), per T26: user-authored configuration. The
-- natural key (user_id, name) is a PARTIAL unique index (WHERE deleted_at
-- IS NULL) so a soft-deleted type never blocks re-creating one with the
-- same name — the same pattern as idx_contacts_vcard_uid_user /
-- idx_cadence_policies_user_entity.
--
-- protocol allows an empty string (not NULL): a handful of seeded defaults
-- (Discord, Wire, Session, GroupMe, Slack) have no stable public
-- profile-by-handle URL, so they seed with an empty template rather than
-- blocking on getting every one perfectly right — the ticket's own call.
-- An empty protocol simply means "not resolvable to a link yet," same as
-- no match at all.

CREATE TABLE link_field_types (
    id TEXT PRIMARY KEY,
    created_at DATETIME,
    updated_at DATETIME,
    deleted_at DATETIME,
    user_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    protocol TEXT NOT NULL DEFAULT '',
    category TEXT NOT NULL,
    icon TEXT,
    is_default BOOLEAN NOT NULL DEFAULT 0,
    position INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_link_field_types_deleted_at ON link_field_types(deleted_at);
CREATE INDEX idx_link_field_types_user_id ON link_field_types(user_id);
CREATE UNIQUE INDEX idx_link_field_types_user_name
    ON link_field_types(user_id, name)
    WHERE deleted_at IS NULL;
