-- N7 (docs/fork-plan/tickets/29-N7-attachments.md) — file/document attachments
-- per contact.
--
-- The row is metadata; the bytes live on disk under a server-generated UUID
-- filename in the configured attachments directory (ATTACHMENTS_DIR, alongside
-- the photo dir). User-authored content, so this table soft-deletes per T26
-- (deleted_at). File removal from disk is the controllers'/cascade's job (the
-- model has no access to the directory) — a soft-deleted row's file is removed
-- at delete time, and the tombstone keeps the T17 change feed convergent.

CREATE TABLE attachments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    deleted_at DATETIME,
    user_id INTEGER NOT NULL,
    contact_vcard_uid TEXT NOT NULL,
    stored_name TEXT NOT NULL,
    original_name TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_attachments_contact ON attachments(contact_vcard_uid);
CREATE INDEX idx_attachments_user ON attachments(user_id);
