-- Issue #352 — DataDecayPolicy: an opt-in, per-contact reminder to
-- periodically re-verify stored info (address, phone, employer, etc.) is
-- still accurate. Deliberately distinct from cadence_policies (T19), which
-- is about staying in touch, not data freshness (docs/adrs/0026-data-decay.md).
--
-- last_verified_at is a stored column, not derived: unlike cadence (which
-- derives from the existing activities timeline), there is no existing
-- "I verified this" event type, so the timestamp has nowhere else to live.
-- NULL means "never verified since creation" — health derivation falls back
-- to created_at as the baseline.
--
-- Soft-delete (deleted_at), per T26: user-authored content, same shape as
-- CadencePolicy. The natural key (user_id, entity_id) is a PARTIAL unique
-- index (WHERE deleted_at IS NULL) so a soft-deleted policy never blocks
-- re-creating one for the same contact.

CREATE TABLE data_decay_policies (
    id TEXT PRIMARY KEY,
    created_at DATETIME,
    updated_at DATETIME,
    deleted_at DATETIME,
    user_id INTEGER NOT NULL,
    entity_id TEXT NOT NULL,
    interval_days INTEGER NOT NULL,
    last_verified_at DATETIME,
    active BOOLEAN NOT NULL DEFAULT 1,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_data_decay_policies_deleted_at ON data_decay_policies(deleted_at);
CREATE INDEX idx_data_decay_policies_entity_id ON data_decay_policies(entity_id);
CREATE INDEX idx_data_decay_policies_feed ON data_decay_policies(user_id, updated_at, id);
CREATE INDEX idx_data_decay_policies_user_id ON data_decay_policies(user_id);
CREATE UNIQUE INDEX idx_data_decay_policies_user_entity
    ON data_decay_policies(user_id, entity_id)
    WHERE deleted_at IS NULL;
