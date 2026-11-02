-- Issue #2004, ADR 0040 slice (c): a contact's image gallery.
--
-- Same row shape as note/activity images; the owner key is
-- contact_vcard_uid (the key Attachment already uses). contacts.main_image_id
-- points at the chosen gallery image and is cleared if that row is deleted.
-- Additive: existing contacts, photos, attachments and note/activity images
-- are untouched.

CREATE TABLE contact_images (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    deleted_at DATETIME,
    user_id INTEGER NOT NULL,
    uuid TEXT NOT NULL,
    contact_vcard_uid TEXT NOT NULL,
    stored_name TEXT NOT NULL,
    thumb_name TEXT NOT NULL,
    mime_type TEXT NOT NULL,
    thumb_mime_type TEXT NOT NULL,
    width INTEGER NOT NULL,
    height INTEGER NOT NULL,
    size_bytes INTEGER NOT NULL,
    thumb_size_bytes INTEGER NOT NULL,
    sensitivity TEXT NOT NULL DEFAULT 'normal',
    dhash INTEGER,
    taken_at DATETIME,
    taken_date TEXT,
    crop_x REAL,
    crop_y REAL,
    crop_w REAL,
    crop_h REAL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX idx_contact_images_uuid ON contact_images(uuid);
CREATE INDEX idx_contact_images_user_id ON contact_images(user_id);
CREATE INDEX idx_contact_images_contact_vcard_uid ON contact_images(contact_vcard_uid);
CREATE INDEX idx_contact_images_deleted_at ON contact_images(deleted_at);
CREATE INDEX idx_contact_images_sensitivity ON contact_images(sensitivity);

ALTER TABLE contacts ADD COLUMN main_image_id INTEGER REFERENCES contact_images(id) ON DELETE SET NULL;
CREATE INDEX idx_contacts_main_image_id ON contacts(main_image_id);
