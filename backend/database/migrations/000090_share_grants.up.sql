-- Issue #2289, ADR 0042 decisions 2, 8 and 13: standing shares.
--
-- share_grants is the authorization record for a live, read-only share of one
-- entity from one user to another. It stores who may read what, never content:
-- no payload column, no snapshot. It deliberately has NO deleted_at (ADR 0004):
-- status is its lifecycle, terminal rows (declined/revoked/expired) are kept for
-- the retention window and then hard-deleted, and DeleteUser removes them.
-- This is separate from the frozen one-time contact_shares table, which is
-- untouched.
--
-- share_blocks is an edge row (blocker -> blocked); hard delete.
--
-- Additive: no existing table or row changes.

CREATE TABLE share_grants (
    id TEXT PRIMARY KEY,
    from_user_id INTEGER NOT NULL,
    to_user_id INTEGER NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    sections TEXT NOT NULL DEFAULT '[]',
    reconfirm_sections TEXT NOT NULL DEFAULT '[]',
    status TEXT NOT NULL DEFAULT 'pending',
    responded_at DATETIME,
    expires_at DATETIME,
    revoked_at DATETIME,
    revoked_reason TEXT,
    revision INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME,
    updated_at DATETIME,
    FOREIGN KEY (from_user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (to_user_id) REFERENCES users(id) ON DELETE CASCADE,
    CHECK (from_user_id <> to_user_id),
    CHECK (entity_type IN ('contact', 'note', 'activity', 'circle')),
    CHECK (status IN ('pending', 'active', 'declined', 'revoked', 'expired', 'blocked')),
    CHECK (revoked_reason IS NULL OR revoked_reason IN ('owner_revoked', 'entity_deleted', 'recipient_deleted'))
);

CREATE INDEX idx_share_grants_to_user_status ON share_grants(to_user_id, status);
CREATE INDEX idx_share_grants_from_user_status ON share_grants(from_user_id, status);
CREATE INDEX idx_share_grants_entity_status ON share_grants(entity_type, entity_id, status);

-- At most one live grant per (sender, recipient, entity); terminal history rows
-- coexist with it (ADR 0004's partial-index form).
CREATE UNIQUE INDEX idx_share_grants_live_unique
    ON share_grants(from_user_id, to_user_id, entity_type, entity_id)
    WHERE status IN ('pending', 'active', 'blocked');

CREATE TABLE share_blocks (
    blocker_user_id INTEGER NOT NULL,
    blocked_user_id INTEGER NOT NULL,
    created_at DATETIME,
    PRIMARY KEY (blocker_user_id, blocked_user_id),
    FOREIGN KEY (blocker_user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (blocked_user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX idx_share_blocks_blocked_user ON share_blocks(blocked_user_id);
