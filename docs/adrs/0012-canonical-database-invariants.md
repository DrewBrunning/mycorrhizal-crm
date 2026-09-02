# ADR 0012: Canonical database invariants

- **Status:** accepted
- **Date:** 2026-09-01
- **Depends on:** ADR 0001 (neutral hub-and-spoke contact model), ADR 0004 (soft vs hard delete
  semantics), ADR 0006 (monotonic per-row revision tokens), ADR 0008 (conditional-write
  enforcement), ADR 0009 (REST write-conflict policy), ADR 0010 (idempotency keys), ADR 0011
  (scheduled-job catch-up)
- **Implements:** issue #493 (DB-02). The `v0.6.8` gate (#538) criterion covered here: *critical
  database invariants are explicitly documented* and *citable*.
- **Feeds:** #460 (DB-01 — the runtime data-integrity checker implements one detection pass per
  invariant below) and #494 (DB-03 — one automated test per invariant, proving it holds in normal
  operation and that the checker fires when it does not).

## Context

The `v0.6.8` milestone asks a question `PRAGMA integrity_check` cannot answer: *is the data
meaningful?* A database can be intact at the page level while holding a relationship that points at a
contact that no longer exists, a `FieldValue` whose definition was removed, or an attachment row
whose file is gone. Before this ADR the properties that must always hold were spread across model
doc-comments, `/CLAUDE.md` traps, six other ADRs, and unwritten convention — so neither #460 (the
checker) nor #494 (the tests) had a single list to implement against.

This ADR is that list. It adds no enforcement code itself; the checker it specifies shipped in #460
(see **Implemented by**, below) and the deeper per-invariant suite is #494. For each invariant it
records: the exact statement, why it must hold, what maintains it today (`file:line`), how it can
still be violated — the specification the checker is written against — and what now checks it.

### Two families, checked differently

- **Data invariants (INV-D\*)** are properties of the database *at rest*. A single read pass over a
  quiescent database can decide whether each holds. These are what #460's application-invariant pass
  checks, and what #494 tests table-driven against a real migrated schema.
- **Application invariants (INV-A\*)** are properties of an *operation* — durability, atomicity,
  idempotency, convergence. They are only observable across a mutation, so #494 tests them with the
  fault-injection seams (#434) and the property suite (#435), not a static scan.

### Structural facts that shape the list

- **Foreign keys are enforced.** `openDSN` sets `_pragma=foreign_keys(1)`
  ([`database/migrate.go:68`](../../backend/database/migrate.go)), pinned by `TestForeignKeysEnforced`
  ([`database/migrate_test.go:144`](../../backend/database/migrate_test.go)) and
  `TestForeignKeyCascadeDeletesOrphanedChildRows`
  ([`database/migrate_test.go:160`](../../backend/database/migrate_test.go)). So a **hard-deleted**
  parent cannot leave a dangling child on any column that is a real SQL `FOREIGN KEY`.
- **Soft deletes bypass foreign keys entirely.** A soft-deleted `Contact` still satisfies every FK
  while being logically gone (ADR 0004). This is the principal source of invalid-but-valid state
  here.
- **Graph endpoints are not FK columns.** `RelationshipEdge.SourceID`/`TargetID`,
  `ExternalIdentity.EntityID`, `ImportSourceLink.EntityUID` reference `Contact.VCardUID` by value,
  not by a declared FK ([`models/relationship_edge.go:67`](../../backend/models/relationship_edge.go)).
  SQLite enforces nothing for them; a manual cleanup step is the only thing that keeps them honest.

## Decision

The following are the canonical database invariants. Each has a stable ID (`INV-D1`…`INV-D9`,
`INV-A1`…`INV-A6`) that #460 and #494 cite.

**Implemented by (DB-01, issue #460):** every at-rest invariant below (`INV-D1`…`INV-D8` and the
cheap `INV-A5` count) has a detection probe in
[`backend/services/data_integrity_service.go`](../../backend/services/data_integrity_service.go),
keyed by a stable `Check` slug and its ADR ID. `RunDataIntegrityChecks` runs them read-only and
per-user-scoped; the storage pass (`PRAGMA integrity_check` + `PRAGMA foreign_key_check`) lives in
[`db_integrity_service.go`](../../backend/services/db_integrity_service.go). Operators reach both
through `GET /api/v1/admin/integrity-check` and the `doctor` CLI
([`backend/cmd/doctor`](../../backend/cmd/doctor)); the scheduled `db_integrity_check` job records a
distinct `data_integrity_check` result and the deep `/health` / `/admin/diagnostics` surfaces show it
separately. Repair is separate and dry-run-by-default
([`data_integrity_repair.go`](../../backend/services/data_integrity_repair.go)), and only removes
truly-orphaned hard-delete join/edge rows. Per-invariant "break the check" tests are in
`data_integrity_service_test.go` / `data_integrity_repair_test.go`. `INV-D9` (the deep,
contract-aware FTS comparison) is issue #462's — its probe is `services.CheckSearchIndexConsistency`
([`search_consistency_service.go`](../../backend/services/search_consistency_service.go)), folded
into the same scheduled job and the same `/admin/diagnostics` sweep.

**Verified by (DB-03, issue #494):** every invariant below has both a *holds* case and a
*detected* case, and the "break the code" step is a committed test rather than a manual ritual
(`/CLAUDE.md`: a test that has never failed has proven nothing).

- **`INV-D1`…`INV-D8`** — a table-driven matrix
  ([`backend/services/data_integrity_matrix_test.go`](../../backend/services/data_integrity_matrix_test.go))
  drives every probe from the full TEST-02 pathological fixture (#430): the untouched fixture is
  clean (no false positives), and one surgical mutation per invariant is detected, named with the
  right ADR id, and adds exactly one new violation class. Hand-verified by neutering each of the ten
  probes and confirming only its rows fail.
- **`INV-D9`** — the FTS consistency probe ships with issue #462's own per-class tests
  ([`services/search_consistency_service_test.go`](../../backend/services/search_consistency_service_test.go):
  `DetectsMissingFromIndex` / `DetectsOrphanInIndex` / `DetectsContentMismatch` /
  `DetectsScopeMismatch`, `CleanCorpusReportsClean`, `SoftDeletedAndArchivedAreNotDivergence`,
  `HandVerify`).
- **Across operations** — the invariants still hold after contact merge (#433), a full delete
  cascade, source import, a migration-chain re-run and a backup/restore round-trip
  ([`data_integrity_operations_test.go`](../../backend/services/data_integrity_operations_test.go),
  [`controllers/data_integrity_operations_test.go`](../../backend/controllers/data_integrity_operations_test.go));
  a contact left behind by a skipped cascade line is named by the checker, turning `/CLAUDE.md` trap
  #6 into a failing test from the checker's side of `TestDeleteCascadeCoverage` (#611).
- **`INV-A1`, `INV-A2`, `INV-A4`, `INV-D8`** — generative properties
  ([`backend/internal/propertytest/data_invariant_property_test.go`](../../backend/internal/propertytest/data_invariant_property_test.go)):
  committed state survives a reopen; a cancelled import commits nothing; every import record is
  accounted for; the flat `contacts.*` projection is a fixpoint of a plain re-save.
- **`INV-A3`** — `TestIdempotentMutation_RetryIsFixpoint`
  ([`internal/propertytest/idempotency_property_test.go`](../../backend/internal/propertytest/idempotency_property_test.go)):
  `retry(op) ≡ op` for every registered idempotent mutation.
- **`INV-A5`** — `TestSearchIndex_RebuildMatchesIncremental`
  ([`services/search_index_property_test.go`](../../backend/services/search_index_property_test.go)):
  a from-scratch `RebuildSearchIndex` produces an index identical to the trigger-maintained one.
- **`INV-A6`** — `TestDataInvariant_A6_ContactSyncConvergesToAFixpoint`
  ([`services/data_integrity_sync_property_test.go`](../../backend/services/data_integrity_sync_property_test.go)):
  re-syncing an unchanged remote is a no-op with stable revision tokens, and a remote change applied
  twice converges to an identical state.

---

### Data invariants

#### INV-D1 — Relationships reference real, owned entities

> Every `RelationshipEdge.SourceID` and `.TargetID` is the `VCardUID` of a `Contact` that exists and
> belongs to the same `UserID` as the edge.

- **Holds because:** `resolveRelationshipEndpoint` verifies both endpoints on create and update — an
  existing id is looked up `WHERE vcard_uid = ? AND user_id = ?` and a missing row is a `404`
  ([`controllers/relationship_edge_controller.go:49`](../../backend/controllers/relationship_edge_controller.go));
  a thin-contact endpoint is created in the same transaction. `SourceID`/`TargetID` carry
  `validate:"required,uuid4"` so a malformed value never reaches the database
  ([`models/relationship_edge.go:70`](../../backend/models/relationship_edge.go)).
- **Can be violated by:** a soft-deleted target (FKs do not see it; `resolveRelationshipEndpoint`'s
  `First` does not filter `deleted_at`, and nothing re-checks an edge after its endpoint is deleted);
  a direct import/merge path that writes an edge without going through `resolveRelationshipEndpoint`;
  a `Contact` hard-deleted out from under an edge by a path not in the cascade list (INV-D3).
- **Checked by:** `checkRelationshipEndpoints` (`data_integrity_service.go`) reports
  `relationship_edge.endpoint_missing` (no `Contact` row at all, INV-D1) and
  `relationship_edge.endpoint_soft_deleted` (INV-D7) per user, via one raw-SQL pass that keeps
  soft-deleted rows visible. `TestDataIntegrity_INV_D7_SoftDeletedEndpoint` seeds the FK-invisible
  case; the DB-03 matrix (`data_integrity_matrix_test.go`) drives both the missing and
  soft-deleted cases from the full TEST-02 fixture and asserts each is the *only* new violation.

#### INV-D2 — Reciprocal relationships are consistent

> The inverse of a stored relationship is always derivable and unambiguous; there is never a stored
> reciprocal edge that could disagree with its partner.

- **Holds because:** only one direction of an edge is ever persisted; the reciprocal is derived
  through `InverseRelationType`, never stored
  ([`models/relationship_edge.go:49`](../../backend/models/relationship_edge.go),
  [`models/relationship_type_registry.go:151`](../../backend/models/relationship_type_registry.go)).
  The registry is total over the token set the `relation_type` validator accepts
  ([`middleware/validation.go:30`](../../backend/middleware/validation.go)), and every entry's
  `Inverse` is itself a registered token whose own `Inverse` points back — pinned by
  `TestRelationTypeRegistryInversesAreConsistent`
  ([`models/relationship_edge_test.go:73`](../../backend/models/relationship_edge_test.go)).
- **Can be violated by:** a registry edit that adds a token with an `Inverse` that is not itself
  registered, or an asymmetric pair (`parent_of`/`child_of`) whose two rows stop being mutual
  inverses; a stored edge whose `Type` is not in the registry (bypassing the validator via a raw
  write or a migration).
- **Checked by:** `checkRelationshipRegistry` asserts `InverseRelationType(InverseRelationType(t)) == t`
  for every `KnownRelationTypes()` token (`relationship_type.registry_inconsistent`) and flags any
  stored `relationship_edges.type` the registry does not know (`relationship_edge.unknown_type`).

#### INV-D3 — No orphaned relationship or join rows

> No `RelationshipEdge`, `CircleMember`, `ContactTag`, `HouseholdMember`, `ContactSyncLink`,
> `CalendarEventLink`, or `FieldValue` row survives the deletion of an entity it depends on.

- **Holds because:** these are hard-delete edge/join rows (ADR 0004, `/CLAUDE.md` trap #7).
  `DeleteContact` enumerates every dependent table explicitly, including the
  `(source_id = ? OR target_id = ?)` edge sweep
  ([`controllers/contact_controller.go:743`](../../backend/controllers/contact_controller.go)); the
  enumeration is compared against the live schema by `TestDeleteCascadeCoverage`
  ([`controllers/delete_cascade_coverage_test.go:174`](../../backend/controllers/delete_cascade_coverage_test.go)),
  which exists because an audit once found 14 tables the manual list had missed. Rows keyed on a
  hard-deleted parent by real FK are removed by SQLite (`ON DELETE CASCADE`, INV-D1 structural note).
- **Can be violated by:** a partially-applied cascade (a delete transaction that failed after some
  but not all dependent tables — see INV-A2); a new join entity added without a line in
  `deleteContactAssociations`; the `VCardUID`-keyed rows, which no FK cascade touches.
- **Checked by:** `checkOrphanedContactRefs` counts `circle_members` / `household_members` /
  `contact_tags` / `field_values` rows whose contact (`*_vcard_uid` / `entity_id`) resolves to no
  row (`join_row.orphaned`, repairable) or a soft-deleted one (INV-D7). The parent-entity side
  (`circle_id`, `tag_id`, …) is FK-backed and covered by the storage pass's `foreign_key_check`.

#### INV-D4 — No dangling external or cross-table references

> Every reference a row holds to another row or to an on-disk file resolves: `ExternalIdentity` /
> `ImportSourceLink` → a live `Contact`; `FieldValue` → a live `FieldDefinition`; `Attachment` /
> profile photo → a file that exists; audit rows → the entity they describe (or an explicit
> tombstone).

- **Holds because:** these references are written only by handlers that first resolve the target
  under `user_id` scope; `FieldValue` writes go through the field-definition service, `Attachment`
  rows are written after the file lands in the photo/attachment store. `ExternalIdentity.EntityID`
  and `ImportSourceLink.EntityUID` carry `uuid4` validation
  ([`models/external_identity.go:47`](../../backend/models/external_identity.go),
  [`models/import_source_link.go:57`](../../backend/models/import_source_link.go)).
- **Can be violated by:** any of the targets being deleted without the referencing row being cleaned
  (the `VCardUID`-keyed links have no FK); a file removed from disk (backup restore, operator error,
  a failed write); a `FieldDefinition` removed while values remain; the audit log deliberately
  outliving its subjects (append-only, `/CLAUDE.md` — a tombstone, not a violation, but the checker
  must distinguish the two).
- **Checked by:** `checkDanglingExternalRefs` (`external_identity` / `external_activity` /
  `import_source_link` → contact), `checkDanglingFieldValues` (`field_value.dangling_definition`,
  repairable), and `checkMissingFiles` (`attachment.missing_file`, `contact.missing_photo_file` —
  skipped when the dir is unset). `checkVanishedAuditRefs` reports `audit_event.vanished_contact` as
  `info`, since an append-only audit row legitimately outlives a soft-deleted contact.

#### INV-D5 — Identifiers are stable

> A primary key, once assigned, is never rewritten for the life of the row: `Contact.VCardUID`, the
> UUID PKs (`RelationshipEdge`, `Household`, `Circle`, `Tag`, `LifeEvent`, `FieldValue`,
> `ExternalIdentity`), and the uint autoincrement PKs.

- **Holds because:** UUID PKs are generated exactly once in `BeforeCreate` and only when empty —
  `if c.VCardUID == "" { c.VCardUID = uuid.New().String() }`
  ([`models/contact.go:431`](../../backend/models/contact.go),
  [`models/relationship_edge.go:115`](../../backend/models/relationship_edge.go)). Update handlers
  use explicit field allowlists (no mass assignment, `/CLAUDE.md` security posture), and no allowlist
  includes the id column. The mutable `revision`/`etag` tokens are a *separate* column so that
  optimistic-concurrency churn never touches identity (ADR 0006).
- **Can be violated by:** a raw `UPDATE` or a migration that rewrites a key column without carrying
  references with it; `AutoMigrate` on a UUID-PK entity (adds a conflicting `ID uint`, `/CLAUDE.md`
  trap #7).
- **Checked by:** primarily a migration-review and schema-dump concern (`cmd/genschema`, issue #529);
  `checkRequiredFields` also reports `contact.missing_vcard_uid` (a blank stable id, DB-03 matrix
  row), and the INV-A1 durability property asserts every contact's `vcard_uid` is byte-identical
  after a close/reopen.

#### INV-D6 — Required fields are present and valid

> Every persisted row satisfies its `NOT NULL` / `CHECK` constraints and the field-level validation
> its create/update path declares (enum membership, `uuid4`, `phone`, `birthday`, `safeurl`,
> `relation_type`, …).

- **Holds because:** request bodies pass `ValidateJSONMiddleware` / `GetValidated[T]` before a
  handler runs ([`middleware/validation.go:318`](../../backend/middleware/validation.go),
  [`middleware/validation.go:332`](../../backend/middleware/validation.go)); custom validators are
  registered in `middleware/validation.go:23`. The schema's own `NOT NULL` / `CHECK` constraints
  (hand-written migration SQL) are the backstop, and enum vocabularies are mirrored into `CHECK`
  constraints (e.g. `ImportRun.Format`, [`models/import_run.go:46`](../../backend/models/import_run.go)).
- **Can be violated by:** a write path that skips the middleware (internal service code, import,
  migration backfill) and constructs a struct by hand; a `CHECK` constraint that drifts from the Go
  `oneof` tag (`/CLAUDE.md` frontend trap #4 is the same drift class).
- **Checked by:** `checkRequiredFields` flags `relationship_edges` rows whose `status` / `source` /
  `sensitivity` is out of range or whose `type` is empty (`relationship_edge.invalid_enum`), and
  contacts with a blank `vcard_uid` (`contact.missing_vcard_uid`, also INV-D5).
  `RunStorageIntegrityChecks` runs `PRAGMA foreign_key_check` for the referential half.

#### INV-D7 — Deleted entities are not active relationship targets

> A soft-deleted `Contact` is not the `TargetID`/`SourceID` of a `confirmed` `RelationshipEdge`, nor
> a live `CircleMember` / `HouseholdMember` / `ContactTag`, nor a projected node in any graph or
> household suggestion.

- **Holds because:** `DeleteContact` removes the contact's edges and join rows in the same
  transaction as the soft delete
  ([`controllers/contact_controller.go:743`](../../backend/controllers/contact_controller.go)); the
  projection step only reads `status = "confirmed"` edges and excludes above-`normal` sensitivity
  ([`models/relationship_edge.go:101`](../../backend/models/relationship_edge.go), ADR 0001's
  deliberately-lossy export rule); `suggested` edges are never treated as fact
  ([`models/relationship_edge.go:27`](../../backend/models/relationship_edge.go)).
- **Can be violated by:** exactly the INV-D1/INV-D3 gaps — this invariant is their user-visible
  consequence. It is called out separately because it is the one the milestone names explicitly
  ("deleted records cannot unexpectedly remain active") and the one a checker should phrase in those
  terms.
- **Checked by:** the `*.endpoint_soft_deleted` / `*.soft_deleted_contact` findings emitted by
  `checkRelationshipEndpoints` and `checkOrphanedContactRefs` — a soft-deleted contact that is still
  a `confirmed` edge endpoint or a live membership. Asserted with `Unscoped()` (`/CLAUDE.md` trap
  #6). Never auto-repaired: the contact may be undeleted.

#### INV-D8 — Canonical records are internally consistent

> `Contact.Card` (the nested `contactmodel.Record`, persisted as a JSON column) is the source of
> truth for standardized contact data; the flat `contacts.*` columns are a faithful projection of
> it; and `Card` itself is well-formed — valid JSON, element `ID`s unique within each collection,
> `PROP-ID`/JSContact-key round-trip preserved.

- **Holds because:** the flat columns are derived from `Card` in `BeforeSave` via `mergeRecordFromFlat`
  — which merges the flat derivation *onto* the loaded `Card` rather than replacing it, so
  no-flat-home data (`SpeakToAs`, `PersonalInfo`, projections) survives a plain `db.Save`
  ([`models/contact.go:356`](../../backend/models/contact.go),
  [`models/contact_card_merge.go:78`](../../backend/models/contact_card_merge.go); T75). `Card` is
  read back with `RecordForContact` (reads what is persisted), never `RecordFromContact` (rebuilds
  from flat fields and silently drops the rest) — `/CLAUDE.md` trap #3
  ([`models/contact_record.go:50`](../../backend/models/contact_record.go)). Element `ID`s serialize
  (`json:"id,omitempty"`) so the round-trip survives save/reload (ADR 0001).
- **Can be violated by:** a new plain-save path that reintroduces a straight `db.Save` on a loaded
  contact without the merge (`/CLAUDE.md` trap #3 standing rule); an import or migration that writes
  `Card` JSON directly with duplicate element IDs or a flat column that disagrees with the nested
  value.
- **Checked by:** `checkCanonicalRecords` streams live contacts in pages and reports
  `canonical_record.invalid_json` (the `card` column does not parse) and
  `canonical_record.duplicate_element_id` (a Card collection has two elements with one `id`, breaking
  the round-trip). The flat-vs-nested reprojection comparison is DB-03's
  `TestDataInvariant_D8_FlatProjectionIsAFixpoint` — over generated records the flat `contacts.*`
  columns are a fixpoint of a plain re-save (it compares the flat columns only; a plain `db.Save`
  still drops a `Media` entry whose photo was never downloaded to `contacts.photo`, a separate
  T75-adjacent bug tracked outside this ADR).

#### INV-D9 — The FTS search index matches canonical data, modulo its contract

> Every live (`deleted_at IS NULL`) `contacts` / `notes` / `activities` row is present in its FTS5
> index (`contacts_fts` / `notes_fts` / `activities_fts`) with the indexed columns and `user_id`
> equal to the base row's; no index row references a base row that no longer exists. The index is
> **not** authoritative on deletion or archive state — a soft-deleted or archived row *may or may
> not* sit in the index, and that is not a divergence.

- **The contract, precisely** (SEARCH-02, issue #462). The index is derived data maintained by SQL
  triggers ([`000007_search_fts5.up.sql:44-59`](../../backend/database/migrations/000007_search_fts5.up.sql),
  extended by `000010`/`000020`). It promises *searchability of live rows*, nothing about deleted or
  archived ones: search correctness for those is enforced by the query, not the index —
  `applyContactSearch`'s FTS subquery "is not trusted to filter them — the outer query's existing
  `archived` Where and GORM's default soft-delete scope do that"
  ([`controllers/contact_controller.go:151-165`](../../backend/controllers/contact_controller.go)).
  The triggers are deliberately asymmetric about deletion: `*_fts_au` re-inserts only
  `WHERE new.deleted_at IS NULL` (a soft-delete drops the row from the index), while `*_fts_ai` has
  no such guard (a row inserted already-soft-deleted — a bulk import, a hand-written migration —
  stays). Both are acceptable; it just means the index's soft-delete content depends on which path
  wrote it. **Decision (issue #462 action 7): the consistency check and the rebuild-equivalence
  property normalise this in the comparison — they only ever compare the index against *live* base
  rows — rather than aligning the `*_fts_ai` triggers with a migration.** Aligning them is a possible
  future tightening, tracked in #463 (SEARCH-03, mutation paths).
- **Holds because:** the triggers fire on every ordinary INSERT/UPDATE/DELETE
  ([`000007_search_fts5.up.sql`](../../backend/database/migrations/000007_search_fts5.up.sql)); the
  rebuild ([`services/search_service.go` `RebuildSearchIndex`](../../backend/services/search_service.go),
  runnable as `cmd/backfill-search-index` or `POST /admin/search/rebuild`) inserts exactly the live
  rows, so a rebuild always converges on what the triggers would have produced (INV-A5).
- **Can be violated by:** a raw-SQL migration or bulk import that updates a base table's indexed
  columns without going through the triggers (stale content); a hard delete a path outside the
  cascade list performed with triggers somehow bypassed (orphan index row); a scoping bug that
  writes a wrong `user_id` into the index; a schema change that indexes a new column without adding
  it to `RebuildSearchIndex` and `ftsConsistencySpecs`.
- **Checked by:** `services.CheckSearchIndexConsistency`
  ([`services/search_consistency_service.go`](../../backend/services/search_consistency_service.go)) —
  read-only, all three indexes, contract-aware (never reports a soft-deleted or archived row in the
  index), reporting each of four classes per row: `missing_from_index`, `orphan_in_index`,
  `content_mismatch` (with column names), `scope_mismatch`. Folded into the scheduled DB-integrity
  job and the on-demand `/admin/diagnostics` sweep (issue #423), not a third schedule. #460 (DB-01)
  picks it up as one of its application-invariant passes; issue #462 ships a per-class holds/detected
  suite ([`services/search_consistency_service_test.go`](../../backend/services/search_consistency_service_test.go));
  the `TestSearchIndex_RebuildMatchesIncremental` property
  ([`services/search_index_property_test.go`](../../backend/services/search_index_property_test.go))
  asserts rebuild-vs-incremental agreement modulo the contract for an arbitrary mutation sequence.

---

### Application invariants

#### INV-A1 — Every successful mutation is durable

> Once a handler returns `2xx`, the change is committed and survives process restart.

- **Holds because:** the store is a single SQLite file in WAL mode; GORM wraps every `Create`/`Save`
  in an implicit transaction, and `_txlock=immediate` makes it take the write lock up front
  ([`database/migrate.go:68`](../../backend/database/migrate.go), `/CLAUDE.md` trap #9). `.Error` is
  checked on every `db.Updates`/`db.Save` (`/CLAUDE.md` trap #4, audited across sites); a handler
  emits `2xx` only after the call returns nil.
- **Can be violated by:** a swallowed `.Error` (the trap-#4 class); a `2xx` written before commit; a
  best-effort side-write mistaken for the mutation (`RecordImportRun` is deliberately best-effort and
  must never be the durability boundary, [`models/import_run.go:72`](../../backend/models/import_run.go)).
- **Checked by:** `TestDataInvariant_A1_CommittedStateSurvivesReopen`
  ([`internal/propertytest/data_invariant_property_test.go`](../../backend/internal/propertytest/data_invariant_property_test.go)):
  a generated batch of creates and updates, then close the connection and reopen the file the way a
  restart would — the whole-database content fingerprint and the contact set are unchanged.

#### INV-A2 — A failed mutation leaves the canonical model unchanged

> A mutation that fails part-way commits nothing — no partial multi-table write is visible.

- **Holds because:** multi-step mutations run inside `gorm.DB.Transaction`, which rolls back on any
  returned error and returns the closure's error verbatim so a typed `*apperrors.AppError` keeps its
  status ([`controllers/relationship_edge_controller.go`](../../backend/controllers/relationship_edge_controller.go),
  `/CLAUDE.md` trap #8); `_txlock=immediate` avoids the deferred-to-write upgrade failure mid-txn.
- **Can be violated by:** a cascade or import loop that does N separate top-level writes instead of
  one transaction (INV-D3's "partially-applied cascade" gap); a handler that catches an error and
  continues.
- **Checked by:** `TestDataInvariant_A2_CancelledImportCommitsNothing` — a source import runs in one
  `ctx`-bound transaction; cancelling it at a random point (from the `progress` callback) leaves the
  database fingerprint byte-identical to before. The example-seam form is
  `TestConfirmInjectedDBErrorFailsClosed` ([`services/import_fault_injection_test.go`](../../backend/services/import_fault_injection_test.go),
  #434). A skipped delete-cascade line is caught by
  `TestDataIntegrity_DetectsSkippedDeleteCascade` ([`controllers/data_integrity_operations_test.go`](../../backend/controllers/data_integrity_operations_test.go)).

#### INV-A3 — An idempotent-op retry yields the same logical result

> Replaying a mutation carrying the same `Idempotency-Key`, or a conditional write that lost the
> race, does not double-apply and does not silently clobber.

- **Holds because:** one idempotency mechanism — a client-supplied `Idempotency-Key`, first response
  replayed for a repeat key (ADR 0010, [`models/idempotency_key.go`](../../backend/models/idempotency_key.go));
  conditional writes require `If-Match` and return `412`/`409` rather than merging (ADR 0008, ADR
  0009). Derived-index rebuilds are idempotent by construction (INV-A5).
- **Can be violated by:** a mutating endpoint that does not consult the key store; a retry window
  shorter than a client's; a conditional-write path that falls back to last-write-wins.
- **Checked by:** `TestIdempotentMutation_RetryIsFixpoint`
  ([`internal/propertytest/idempotency_property_test.go`](../../backend/internal/propertytest/idempotency_property_test.go)):
  for every registered idempotent mutation, `retry(op)` leaves the database fingerprint unchanged
  from `op`. A stale `If-Match` is rejected, not applied
  ([`controllers/conditional_write_test.go`](../../backend/controllers/conditional_write_test.go),
  [`controllers/idempotency_e2e_test.go`](../../backend/controllers/idempotency_e2e_test.go)).

#### INV-A4 — An import is wholly accepted or explicitly reported partial/unusable

> Every import returns a per-record accounting — created / updated / skipped / errored — and never
> leaves a half-written record with no report of it.

- **Holds because:** `ImportResult` carries the four counts plus per-record `Errors`
  ([`models/import.go:227`](../../backend/models/import.go)); the outcome is persisted as an
  `ImportRun` so an operator can answer "did anything fail?"
  ([`models/import_run.go:30`](../../backend/models/import_run.go)); the source-import framework
  applies per-record add/skip/merge actions and reports each (ADR 0007,
  `ExecuteSourceImportWithActions`). A record that cannot be mapped is reported unusable, not
  dropped.
- **Can be violated by:** an import path that writes a record but omits it from every count; a
  crash mid-import with no `ImportRun` written (best-effort write, INV-A1 note); a mapping that
  partially applies a record and reports it as fully created.
- **Checked by:** `TestDataInvariant_A4_ImportAccountsForEveryRecord` — a generated plan of N valid
  contacts plus k relationships with unresolved endpoints: every valid contact is created, every
  unmappable relationship is named in `report.Issues` (count exact), and the result trips no
  integrity probe. `TestDataIntegrity_HoldsAfterImport`
  ([`services/data_integrity_operations_test.go`](../../backend/services/data_integrity_operations_test.go))
  covers the clean path.

#### INV-A5 — Every derived index is regenerable from canonical data

> The FTS5 search index, the flat `contacts.*` projection, `contacts.sort_name`, and every other
> denormalized artifact can be dropped and rebuilt from the canonical tables with an identical
> result.

- **Holds because:** the FTS index has a standalone rebuild — `RebuildSearchIndex`
  ([`services/search_service.go:331`](../../backend/services/search_service.go)), runnable as
  `cmd/backfill-search-index` and documented as "always the same as what the triggers would have
  produced" ([`backend/cmd/backfill-search-index/main.go`](../../backend/cmd/backfill-search-index/main.go));
  the flat contact columns are recomputed from `Card` by `ApplyRecordToContact` + `BeforeSave`
  ([`models/contact_record_reverse.go:53`](../../backend/models/contact_record_reverse.go)); FTS
  triggers keep the index live between rebuilds (migrations `000007`, `000010`, `000020`).
- **Can be violated by:** a raw-SQL migration or bulk import that bypasses the FTS triggers without a
  follow-up backfill (the reason `cmd/backfill-search-index` exists); a derived artifact with *no*
  rebuild path (#497 owns the inventory of these).
- **Checked by:** `checkDerivedIndexes` (`data_integrity_service.go`) compares `COUNT(*)` of each
  `*_fts` table against its live base rows (`derived_index.fts_row_count_divergent`, repairable via
  `RebuildSearchIndex`) — the cheap version, driven from the TEST-02 fixture by the DB-03 matrix.
  The deep, per-row, contract-aware comparison is INV-D9's `services.CheckSearchIndexConsistency`
  (issue #462); the rebuild-vs-incremental equivalence for an arbitrary mutation sequence is
  `TestSearchIndex_RebuildMatchesIncremental`
  ([`services/search_index_property_test.go`](../../backend/services/search_index_property_test.go)).
  The flat-column family inventory is #497.

#### INV-A6 — Every sync operation eventually converges

> Repeated CardDAV/CalDAV sync of an unchanged remote reaches a fixed point; a remote change is fully
> applied or fully retried, never left half-merged.

- **Holds because:** REST/CardDAV writes are full-overwrite by design — `reconcileContactSync`
  applies a whole address object and diffs membership to find deletions
  ([`services/contact_sync_service.go:393`](../../backend/services/contact_sync_service.go), the T13
  ticket; `/CLAUDE.md` domain notes); the `revision`/`etag` token advances monotonically per write
  so a no-op sync is detectable (ADR 0006, [`models/contact.go:450`](../../backend/models/contact.go));
  missed scheduled sync occurrences catch up once, de-duplicated (ADR 0011).
- **Can be violated by:** a new *two-way* sync path that copies `reconcileContactSync`'s
  discard-local-on-remote-change policy without deciding deliberately (the T13 warning); a partial
  apply of a remote batch with no retry marker; an `etag` derived from wall-clock (ADR 0006's
  rejected alternative).
- **Checked by:** `TestDataInvariant_A6_ContactSyncConvergesToAFixpoint`
  ([`services/data_integrity_sync_property_test.go`](../../backend/services/data_integrity_sync_property_test.go)):
  a generative property over `reconcileContactSync` — re-syncing an unchanged remote is a no-op with
  stable `revision` tokens, and a remote change applied twice converges to a byte-identical state,
  never a half-merge.

## Consequences

- **#460 (DB-01) implements one detection pass per `INV-D*`**, reported distinctly from the
  storage-level `PRAGMA integrity_check` pass, plus `PRAGMA foreign_key_check` on the storage side.
  Detection is separate from repair and ships first; repair is a separate operator-invoked command,
  independently tested (#460 action 4).
- **#494 (DB-03) implements one test per invariant** — see **Verified by**, above — and for each the
  "break the code" step is a committed test, not a manual ritual (`/CLAUDE.md`: a test that has never
  failed has proven nothing). `INV-D*` are a table-driven matrix over the TEST-02 fixture against a
  real migrated schema (`/CLAUDE.md` trap #1) plus an across-operations suite (merge / cascade /
  import / migration / restore); `INV-A*` are generative properties (`internal/propertytest`, the
  #435 suite) with the #434 seams as the example-form backstop.
- **This ADR is the citable list the `v0.6.8` gate (#538) requires.** Each invariant above is
  written down with `file:line` references; "critical database invariants are explicitly documented"
  and "each invariant is written down and citable" are satisfied by this file.
- **Adding a persisted entity or an external reference means adding its invariant here** in the same
  PR — the same rule ADR 0004 states for the delete cascade and `docs/security/data-retention-
  lifecycle.md` states for a new copy of user data.
- **No new invariant is enforced by this ADR.** Where an invariant currently rests on convention or
  a manual step (INV-D1/D3/D4's `VCardUID` links, INV-D7), that is stated as the gap the checker
  closes — not silently smoothed over.
