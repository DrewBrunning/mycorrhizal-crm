-- Issue #2295, ADR 0043 (decisions 1, 2, 5, 6, 9): reusable entities --
-- organisations, places and pets -- shared by many contacts.
--
-- Purely additive: no Card, contact value or existing row is rewritten. Existing
-- ORG/ADR values stay contact values. The only change to an existing table is
-- relaxing the field_definitions key uniqueness from (user_id, key) to
-- (user_id, target, key), which no existing row can violate (every row is
-- target 'contact').

-- ---------------------------------------------------------------------------
-- entities: one table for all kinds. Soft delete (ADR 0004: user-authored).
-- core is the kind-specific typed JSON, encrypted at rest with the same
-- serializer as contacts.card; name_key is derived by the service, never
-- client-supplied.
-- ---------------------------------------------------------------------------
CREATE TABLE entities (
    id TEXT PRIMARY KEY,
    created_at DATETIME,
    updated_at DATETIME,
    deleted_at DATETIME,
    user_id INTEGER NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('organisation', 'place', 'pet')),
    name TEXT NOT NULL CHECK (length(name) > 0 AND length(name) <= 200),
    name_key TEXT NOT NULL DEFAULT '',
    core TEXT NOT NULL DEFAULT '',
    sensitivity TEXT NOT NULL DEFAULT 'normal' CHECK (sensitivity IN ('normal', 'private', 'secret')),
    etag TEXT,
    revision INTEGER NOT NULL DEFAULT 1,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_entities_user_kind ON entities(user_id, kind);
CREATE INDEX idx_entities_user_name_key ON entities(user_id, kind, name_key);
CREATE INDEX idx_entities_feed ON entities(user_id, updated_at, id);
CREATE INDEX idx_entities_deleted_at ON entities(deleted_at);
CREATE INDEX idx_entities_sensitivity ON entities(sensitivity);
-- Lets a link's (entity_id, kind) be a composite foreign key, so a link can
-- never carry a kind different from its entity's.
CREATE UNIQUE INDEX idx_entities_id_kind ON entities(id, kind);

-- kind is immutable after create (a kind change is a delete and re-create).
CREATE TRIGGER entities_kind_immutable
BEFORE UPDATE OF kind ON entities
WHEN NEW.kind <> OLD.kind
BEGIN
    SELECT RAISE(ABORT, 'entities.kind is immutable');
END;

-- ---------------------------------------------------------------------------
-- contact_entity_links: a contact attached to an entity. NOT the relationship
-- graph (ADR 0043 decision 2). Soft delete is a deliberate ADR 0004 exception
-- for a join row: no natural-key unique index exists (re-hire = two links) and
-- the one unique index (primary) is partial on deleted_at IS NULL.
-- period is a JSON contactmodel.TemporalRange; an all-empty range is NULL.
-- ---------------------------------------------------------------------------
CREATE TABLE contact_entity_links (
    id TEXT PRIMARY KEY,
    created_at DATETIME,
    updated_at DATETIME,
    deleted_at DATETIME,
    deletion_batch_id TEXT,
    user_id INTEGER NOT NULL,
    contact_vcard_uid TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('organisation', 'place', 'pet')),
    role TEXT NOT NULL CHECK (
        (kind = 'organisation' AND role IN ('employee', 'owner', 'contractor', 'volunteer', 'member', 'other')) OR
        (kind = 'place' AND role IN ('resident', 'owner', 'visitor', 'other')) OR
        (kind = 'pet' AND role IN ('owner', 'carer', 'other'))
    ),
    role_label TEXT NOT NULL DEFAULT '' CHECK (length(role_label) <= 100),
    is_primary INTEGER NOT NULL DEFAULT 0 CHECK (is_primary IN (0, 1)),
    period TEXT,
    sensitivity TEXT NOT NULL DEFAULT 'normal' CHECK (sensitivity IN ('normal', 'private', 'secret')),
    etag TEXT,
    revision INTEGER NOT NULL DEFAULT 1,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (entity_id, kind) REFERENCES entities(id, kind) ON DELETE CASCADE
);
CREATE INDEX idx_contact_entity_links_user_contact ON contact_entity_links(user_id, contact_vcard_uid);
CREATE INDEX idx_contact_entity_links_entity ON contact_entity_links(entity_id);
CREATE INDEX idx_contact_entity_links_deleted_at ON contact_entity_links(deleted_at);
CREATE INDEX idx_contact_entity_links_batch ON contact_entity_links(deletion_batch_id);
CREATE INDEX idx_contact_entity_links_sensitivity ON contact_entity_links(sensitivity);
-- At most one live primary per contact per kind.
CREATE UNIQUE INDEX idx_contact_entity_links_primary
    ON contact_entity_links(user_id, contact_vcard_uid, kind)
    WHERE is_primary = 1 AND deleted_at IS NULL;

CREATE TRIGGER contact_entity_links_kind_immutable
BEFORE UPDATE OF kind ON contact_entity_links
WHEN NEW.kind <> OLD.kind
BEGIN
    SELECT RAISE(ABORT, 'contact_entity_links.kind is immutable');
END;

-- ---------------------------------------------------------------------------
-- entity_deletion_snapshots: the ADR 0037 mechanism at entity scale. Carries
-- the hard-deleted field values (payload, encrypted) so an entity restore can
-- re-create them. Hard delete: lives exactly as long as the entity is trashed.
-- ---------------------------------------------------------------------------
CREATE TABLE entity_deletion_snapshots (
    id TEXT PRIMARY KEY,
    created_at DATETIME,
    updated_at DATETIME,
    user_id INTEGER NOT NULL,
    entity_id TEXT NOT NULL,
    deletion_batch_id TEXT NOT NULL,
    trashed_at DATETIME NOT NULL,
    payload_version INTEGER NOT NULL,
    payload TEXT NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX idx_entity_deletion_snapshots_entity ON entity_deletion_snapshots(user_id, entity_id);

-- ---------------------------------------------------------------------------
-- dismissed_entity_pairs: ordered id pair the user hid from merge suggestions
-- (the dismissed_duplicate_pairs pattern). Hard delete, covered by the entity
-- cascade.
-- ---------------------------------------------------------------------------
CREATE TABLE dismissed_entity_pairs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    user_id INTEGER NOT NULL,
    id_low TEXT NOT NULL,
    id_high TEXT NOT NULL,
    CHECK (id_low < id_high),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX idx_dismissed_entity_pairs_unique ON dismissed_entity_pairs(user_id, id_low, id_high);

-- ---------------------------------------------------------------------------
-- field_definitions: key uniqueness becomes per (user, target, key) so "notes"
-- can exist on both a contact and an organisation. Only relaxes the constraint.
-- ---------------------------------------------------------------------------
DROP INDEX idx_field_definitions_user_key;
CREATE UNIQUE INDEX idx_field_definitions_user_key ON field_definitions(user_id, target, key);
