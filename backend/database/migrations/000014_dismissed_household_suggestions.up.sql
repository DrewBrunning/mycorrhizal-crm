-- T40 (docs/fork-plan/tickets/49-T40-household-suggestions-shared-address.md) —
-- permanent dismissal memory for address-based household suggestions.
--
--   address_hash = SHA-256 hex of the normalized shared-address key.
--   member_hash  = SHA-256 hex of the sorted member VCardUIDs joined.
--
-- Together the pair uniquely identifies one suggested group; the detection
-- query excludes any group whose (user_id, address_hash, member_hash) triple
-- is present, so dismissing once is dismissing forever (same addresses always
-- produce the same group — deterministic and idempotent). Hard-delete per T26
-- (a join-shaped row whose identity IS its natural key), so there is no
-- deleted_at column and no partial index — the unique index makes duplicates
-- structurally impossible.

CREATE TABLE dismissed_household_suggestions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    user_id INTEGER NOT NULL,
    address_hash TEXT NOT NULL,
    member_hash TEXT NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX idx_dismissed_household_suggestions_unique
    ON dismissed_household_suggestions(user_id, address_hash, member_hash);
CREATE INDEX idx_dismissed_household_suggestions_user_id
    ON dismissed_household_suggestions(user_id);
