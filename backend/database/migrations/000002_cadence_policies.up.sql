-- T19 — CadencePolicy
-- relationship-maintenance rule. Health (next_due / overdue_by) is derived
-- from the timeline, never stored, so this table carries no derived columns.
--
-- Soft-delete (deleted_at), per T26: user-authored content. The natural key
-- (user_id, entity_id) is a PARTIAL unique index (WHERE deleted_at IS NULL)
-- so a soft-deleted policy never blocks re-creating a cadence for the same
-- contact — the same pattern as idx_contacts_vcard_uid_user.
--
-- qualifying_types is a JSON array (TEXT), same serialization as
-- relationship_edges.metadata and life_events.related_entity_ids.

CREATE TABLE cadence_policies (
    id TEXT PRIMARY KEY,
    created_at DATETIME,
    updated_at DATETIME,
    deleted_at DATETIME,
    user_id INTEGER NOT NULL,
    entity_id TEXT NOT NULL,
    target_interval_days INTEGER NOT NULL,
    qualifying_types TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_cadence_policies_deleted_at ON cadence_policies(deleted_at);
CREATE INDEX idx_cadence_policies_entity_id ON cadence_policies(entity_id);
CREATE INDEX idx_cadence_policies_feed ON cadence_policies(user_id, updated_at, id);
CREATE INDEX idx_cadence_policies_user_id ON cadence_policies(user_id);
CREATE UNIQUE INDEX idx_cadence_policies_user_entity
    ON cadence_policies(user_id, entity_id)
    WHERE deleted_at IS NULL;
