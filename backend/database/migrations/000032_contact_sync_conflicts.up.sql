-- Surface CardDAV sync conflicts to the user (issue #395).
--
-- contact_sync_conflicts: one row per local edit a CardDAV contact sync
-- overwrote by documented full-replace policy (reconcileContactSync). The
-- sync service records which field, what the local value was, and what the
-- remote value became, so the UI can show the user "your edit to X was
-- overwritten" and offer the local value back (restore).
--
-- Mirrors reach_out_suggestions' shape (issue #177): system-generated,
-- edge-shaped row, UUID PK generated in BeforeCreate, no deleted_at, no
-- natural-key unique constraint to protect — "dismiss" is a status update,
-- never a delete (CLAUDE.md backend trap 7). Per-user scoped everywhere.
-- Field values are the user's own synced contact data — never the read-time
-- projected view, so private/secret relationship/preference/custom-field
-- content (which is projected at read time only and never stored on the
-- Card) cannot leak into a conflict notice.
CREATE TABLE contact_sync_conflicts (
    id TEXT PRIMARY KEY,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    user_id INTEGER NOT NULL,
    subscription_id INTEGER NOT NULL,
    contact_id INTEGER NOT NULL,
    field TEXT NOT NULL,
    local_value TEXT NOT NULL DEFAULT '',
    remote_value TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'dismissed'))
);

CREATE INDEX idx_contact_sync_conflicts_user_status ON contact_sync_conflicts(user_id, status);
CREATE INDEX idx_contact_sync_conflicts_contact ON contact_sync_conflicts(contact_id);
CREATE INDEX idx_contact_sync_conflicts_subscription ON contact_sync_conflicts(subscription_id);

-- The per-field "last synced value" snapshot on each contact_sync_links row,
-- used by the sync service to tell a local edit (current value != last
-- synced value) from a plain remote change, so only real overwritten local
-- edits surface as conflicts. JSON text; empty for links created before this
-- migration, and those get their baseline on the next sync with no
-- retroactive conflicts.
ALTER TABLE contact_sync_links ADD COLUMN synced_values TEXT NOT NULL DEFAULT '';
