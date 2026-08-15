-- T14 — the
-- generic integration substrate (§91.12): two tables serve every future
-- integration (Immich, Paperless-ngx, Matrix, GitHub, ...) so none of them
-- ever grows bespoke columns.
--
-- external_identities: "this contact IS this thing in that external system."
-- One Contact (by VCardUID — the graph invariant, §90 D3) ↔ many identities,
-- the "identity hub" of §90. System names the integration, external_id is the
-- ID inside it, url is an optional deep-link, metadata is the JSON payload for
-- system-specific data (the RelationshipEdge.Metadata pattern) and sync_status/
-- last_synced_at track the integration's sync lifecycle.
--
-- external_activities: "something that happened in an external system",
-- linkable into the contact's timeline (§91.8). Keyed by system + external
-- activity ID, with the contact it concerns (entity_id), a type
-- (photo-appearance, media-watched, ...), when it happened, and a JSON payload
-- holding the summary/provenance.
--
-- Both are edge/join-shaped rows (a re-sync must never duplicate the same
-- system+external_id pair), so they HARD-delete per T26 — the same rule that
-- keeps every other natural-key unique table in this schema hard-delete. The
-- unique (system, external_id, user_id) constraints are what make a re-sync
-- idempotent: an integration upserts on that key instead of inserting.

CREATE TABLE external_identities (
    id TEXT PRIMARY KEY,
    created_at DATETIME,
    updated_at DATETIME,
    user_id INTEGER NOT NULL,
    entity_id TEXT NOT NULL,
    system TEXT NOT NULL,
    external_id TEXT NOT NULL,
    url TEXT,
    metadata TEXT,
    sync_status TEXT NOT NULL DEFAULT 'idle',
    last_synced_at DATETIME,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX idx_external_identities_system_ext_user
    ON external_identities(system, external_id, user_id);
CREATE INDEX idx_external_identities_entity_id ON external_identities(entity_id);
CREATE INDEX idx_external_identities_system ON external_identities(system);
CREATE INDEX idx_external_identities_user_id ON external_identities(user_id);

CREATE TABLE external_activities (
    id TEXT PRIMARY KEY,
    created_at DATETIME,
    updated_at DATETIME,
    user_id INTEGER NOT NULL,
    entity_id TEXT NOT NULL,
    source_system TEXT NOT NULL,
    external_id TEXT NOT NULL,
    type TEXT NOT NULL,
    occurred_at DATETIME NOT NULL,
    payload TEXT,
    provenance TEXT NOT NULL DEFAULT 'external',
    sync_state TEXT NOT NULL DEFAULT 'synced',
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX idx_external_activities_system_ext_user
    ON external_activities(source_system, external_id, user_id);
CREATE INDEX idx_external_activities_entity_id ON external_activities(entity_id);
CREATE INDEX idx_external_activities_occurred_at ON external_activities(occurred_at);
CREATE INDEX idx_external_activities_type ON external_activities(type);
CREATE INDEX idx_external_activities_user_id ON external_activities(user_id);
