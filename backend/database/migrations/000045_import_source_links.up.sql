-- Import source links (issues #351 + #353 — the Meerkat and Monica import
-- mappings).
--
-- Every row produced by a source import (a Meerkat database, a Monica
-- snapshot, and any future importer that joins the shared framework) is
-- tracked here by its identity in the SOURCE system, so re-running the same
-- import never duplicates: a row whose (system, external_id) pair is already
-- present was imported by an earlier run and is skipped. This is the
-- application of the CON-04 idempotency property (issue #459) to source
-- imports, and it is a uniform ledger for every entity kind — contacts and
-- graph entities alike — instead of one bespoke dedupe mechanism per entity
-- type.
--
-- external_id is the source row's own identity, namespaced per entity kind
-- (e.g. "contact/7", "note/12", "relationship/3"), so two different entity
-- kinds from the same source can never collide even though the natural-key
-- unique index only spans (system, external_id, user_id).
--
-- entity_kind/entity_uid record what the row became locally (kind ∈
-- contact|relationship|household|circle|tag|gift|preference|activity|note|
-- reminder; uid is the local identity — a VCardUID for contacts, the UUID
-- primary key for the UUID-PK entities, or "id:<n>" for the uint-PK rows).
-- They are informational/debugging, not part of the uniqueness contract.
--
-- Hard-delete (no DeletedAt), matching the graph-adjacent join-row class
-- (ADR 0004): a link is a ledger fact, not user-authored content with an undo
-- button, and it must not block re-creating the same source row after a
-- delete. It belongs to the importing user and is removed only by
-- DeleteUser's manual cascade (CLAUDE.md backend trap #6).

CREATE TABLE import_source_links (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    user_id INTEGER NOT NULL,
    system TEXT NOT NULL,
    external_id TEXT NOT NULL,
    entity_kind TEXT NOT NULL,
    entity_uid TEXT NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX idx_import_source_links_system_ext_user
    ON import_source_links(system, external_id, user_id);
CREATE INDEX idx_import_source_links_entity_uid ON import_source_links(entity_uid);
CREATE INDEX idx_import_source_links_user_id ON import_source_links(user_id);
