-- Issue #1994, ADR 0040 slice (a): images attached to an activity.
--
-- One row per stored image, owned by an activity and a user. Image bytes live
-- on disk under server-generated UUID names (IMAGES_DIR); this table is the
-- metadata and ownership record. User-authored content, so it soft-deletes
-- with the activity (ADR 0004, backend trap 7).
--
-- Additive: existing contact attachments, profile photos and activities are
-- untouched.

CREATE TABLE activity_images (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    deleted_at DATETIME,
    user_id INTEGER NOT NULL,
    uuid TEXT NOT NULL,
    activity_id INTEGER NOT NULL,
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
    FOREIGN KEY (activity_id) REFERENCES activities(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX idx_activity_images_uuid ON activity_images(uuid);
CREATE INDEX idx_activity_images_user_id ON activity_images(user_id);
CREATE INDEX idx_activity_images_activity_id ON activity_images(activity_id);
CREATE INDEX idx_activity_images_deleted_at ON activity_images(deleted_at);
CREATE INDEX idx_activity_images_sensitivity ON activity_images(sensitivity);
