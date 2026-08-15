-- T93 —
-- permanent dismissal memory for duplicate-contact pairs.
--
-- The duplicate scanner (services/duplicate_service.go) re-derives candidate
-- pairs on every request, so without a persisted "not a duplicate" record the
-- same twins / father-and-son pair would be re-offered on every visit and the
-- review surface would be noise. One row per dismissed pair, identified by the
-- ORDERED (uid_low, uid_high) pair of Contact.VCardUIDs; ordering means
-- (A,B) and (B,A) can never both be stored.
--
-- Hard-delete per /CLAUDE.md trap #7 (T26): a join-shaped row whose identity
-- IS its natural key, so there is no deleted_at column and no partial index —
-- the unique index makes duplicates structurally impossible. Deleting either
-- contact sweeps its dismissals in DeleteContact's cascade checklist
-- (backend/controllers/contact_controller.go's deleteContactAssociations).

CREATE TABLE dismissed_duplicate_pairs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    user_id INTEGER NOT NULL,
    uid_low TEXT NOT NULL,
    uid_high TEXT NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX idx_dismissed_duplicate_pairs_unique
    ON dismissed_duplicate_pairs(user_id, uid_low, uid_high);
CREATE INDEX idx_dismissed_duplicate_pairs_user_id
    ON dismissed_duplicate_pairs(user_id);
