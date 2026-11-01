-- Issue #2004, ADR 0040 slice (b): images attached to a note.
--
-- Same row shape as activity_images; the owner key is note_id. User-authored
-- content, so it soft-deletes with the note (ADR 0004, backend trap 7).
-- Additive: existing notes, contacts and activity images are untouched.

CREATE TABLE note_images (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    deleted_at DATETIME,
    user_id INTEGER NOT NULL,
    uuid TEXT NOT NULL,
    note_id INTEGER NOT NULL,
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
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (note_id) REFERENCES notes(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX idx_note_images_uuid ON note_images(uuid);
CREATE INDEX idx_note_images_user_id ON note_images(user_id);
CREATE INDEX idx_note_images_note_id ON note_images(note_id);
CREATE INDEX idx_note_images_deleted_at ON note_images(deleted_at);
CREATE INDEX idx_note_images_sensitivity ON note_images(sensitivity);
