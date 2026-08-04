-- P1 (docs/fork-plan/tickets/31-P1-contact-sharing.md) — one-time filtered
-- contact copy shared from one user to another on the same instance. This is
-- NOT a standing/live share (that's P1b, deferred): payload is a frozen
-- T9-filtered JSContact snapshot serialized once at creation time.
--
-- No soft-delete (deleted_at): this ticket has no delete/withdraw endpoint —
-- declining is the "soft" outcome (flips status, row survives so the
-- sender's offer isn't silently destroyed). The only place a row is ever
-- removed is DeleteUser's cascade sweep, hence ON DELETE CASCADE on both
-- user FKs, matching every other user_id FK in 000001_initial_schema.

CREATE TABLE contact_shares (
    id TEXT PRIMARY KEY,
    created_at DATETIME,
    updated_at DATETIME,
    from_user_id INTEGER NOT NULL,
    to_user_id INTEGER NOT NULL,
    contact_display_name TEXT,
    payload TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    responded_at DATETIME,
    FOREIGN KEY (from_user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (to_user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_contact_shares_to_user_feed ON contact_shares(to_user_id, updated_at, id);
CREATE INDEX idx_contact_shares_from_user_feed ON contact_shares(from_user_id, updated_at, id);
CREATE INDEX idx_contact_shares_status ON contact_shares(status);
