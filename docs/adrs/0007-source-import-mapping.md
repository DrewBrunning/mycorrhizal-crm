# ADR 0007: Source imports — Meerkat direct-DB and Monica snapshot, over one shared mapping framework

- **Status:** accepted
- **Date:** 2026-08-29
- **Decides:** issues #353 (Meerkat import mapping) and #351 (Monica import mapping) — the
  *data* half of `v0.6.4`'s import milestone. The user-facing assistants are #550 and #549.

## Context

Two self-hosted personal CRMs carry data users may want to move here:

- **Meerkat** (`fbuchner/meerkat-crm`) — this repo's hard-fork upstream. Its SQLite schema is a
  direct ancestor of this repo's pre-fork shape; its data (contacts, flat relationships, notes,
  activities, reminders, per-contact circles and custom fields) lives entirely in the database
  file.
- **Monica** (`monicahq/monica`) — a separate self-hosted personal CRM with a REST API. Data is
  reachable only through that API (no offline export format); upstream Meerkat already built a
  Monica import assistant (meerkat-crm#211/#216/#218) whose mapping and flow are proven against
  real instances.

Both were previously filed as one-ticket "assistant + UI + mapping" efforts; `v0.6.4` split the
data half out. This ADR settles the two design questions those tickets surface: which data path
each source takes, and whether the two importers share a framework.

## Decision

### 1. Meerkat imports read the SQLite database file directly

Path 1 of #353's "Decide first": the importer opens the Meerkat database read-only, reads the
importable tables, and maps them forward. Path 2 (Meerkat's VCF/CSV export through the existing
`import_session.go` pipeline) was rejected as the primary path because it **loses structure by
construction**: the flat `relationships` table, the per-contact `circles` grouping column, and the
`custom_fields` map exist only in the database, and a VCF export carries none of them. The
milestone's own framing — transform existing data *without silent data loss* — argues the same
way. The reader (`backend/meerkat`) is read-only and tolerant of schema drift, so a deployment at
any Meerkat migration version imports.

What the direct-DB path preserves: relationship graph (as directed edges), circle groupings,
custom fields, food preferences, notes, activities, reminders, soft-delete tombstones as reported
skips, and each contact's own `vcard_uid` (kept, giving cross-system identity stability and a
natural re-import anchor).

What it cannot carry, reported as named losses, never silent:
- **Photos.** Meerkat stores only a filename (`contacts.photo`) pointing at the source server's
  `PROFILE_PHOTO_DIR`; the image bytes are not in the database. There is no way to transfer them
  without the filesystem, so the mapping reports `field: photo, category: unsupported`.
- **Relationships whose target is not a contact row** (a legacy `name` without a
  `related_contact_id`, or a dangling id). Reported `category: unsupported` with the person's
  name.
- **Multi-user ambiguity.** A Meerkat deployment can hold several users; the default import
  targets the first source user and never silently mixes accounts.

VCF/CSV export remains available to the user as an ordinary file import — the direct-DB path is
the *recommended* and default one, not a replacement for the file pipeline.

### 2. Monica imports consume a snapshot, not a live connection — for this ticket

The Monica *mapping* operates on a `monica.Snapshot` — the complete in-memory copy of an account
that the live fetch produces (upstream meerkat-crm#211 defines that shape). This ticket is
backend-mapping-and-fixtures; the live client, pagination, rate limiting, SSRF-guarded transport,
and the assistant UI are the deferred #549 surface. The checked-in fixture
(`testdata/monica-fixture/snapshot.json`) *is* a snapshot, so the mapping is provable today
without a live instance. Porting upstream's proven mapping was the cheapest correctness win
available (the reason #351 points at the three upstream PR diffs); the port re-targets it from the
flat contact model onto the neutral `Record` and maps Monica's graph to real entities rather than
notes.

Monica-specific decisions, all documented in `docs/import/monica-mapping.md`:
- Relationship direction follows Monica's verified semantics (`ApiRelationshipController` +
  `Contact::setRelationship`): `{contact_is: A, of_contact: B, type: "daughter"}` means *B is A's
  daughter*, so the edge is **B → A** with the matched registered type. Monica writes both
  directions of every relationship, so the reciprocal half is collapsed (the lower subject id's
  survives) — the local graph derives the inverse from one stored edge, and importing both halves
  would render each relationship twice.
- Gifts map to real `Gift` records (`offered` → `given`); logged calls map to
  `InteractionTypeCall` activities; tasks and debts become dated notes (no entity home); `is_dead`
  lands as a `death` anniversary; `is_starred` as the CRM `IsFavorite` flag; avatars are carried
  on the plan and downloaded by the assistant (#549), never fetched by the backend mapping.

### 3. One shared mapping framework, not two pipelines

Both mappers produce the same neutral output — an `ImportSourcePlan` (contacts as
`contactmodel.Record` values plus graph entities referenced by source refs) — and one transactional
engine (`services.ExecuteSourceImport`) applies it. This is the deliberate answer to #353's
"generalize one importer framework" recommendation: a third importer (Monica v3, a future Nextcloud
export, ...) is a *mapper*, not a third pipeline. The abstraction cost is paid once; the risk it
guards against — a third importer re-implementing idempotency, ownership scoping, or loss
reporting slightly differently — is the one that actually ships bugs.

The engine's rules are non-negotiable and pinned by tests:

- **Contacts land through `ApplyRecordToContact`** (CLAUDE.md backend trap #2): the mappers
  produce neutral Records, the engine maps them, and only the CRM-local flags with no neutral home
  (`Archived`/`IsFavorite`) are set directly.
- **Nothing is dropped silently.** Every loss/degradation is an `ImportIssue` naming the record,
  the field, and a category over ADR-0002's tiers (`unsupported`, `lossy`, `transformed`,
  `invalid`; `skipped` for policy exclusions like soft-deleted source rows).
- **A failing record leaves nothing behind.** The whole import is one transaction; a contact that
  fails validation is rejected whole, and its dependent graph entities are dropped with a named
  issue, never orphaned.
- **Re-running never duplicates** (CON-04 / #459). Every row produced is recorded in a new
  `import_source_links` table keyed by its source identity `(system, external_id, user_id)`;
  re-running skips rows already imported. The existing `ExternalIdentity` table was considered and
  rejected as the ledger: its `EntityID` is a contact VCardUID, so it cannot track notes,
  reminders, edges, or households, and a uniform ledger for every entity kind is the property the
  milestone actually needs.
- **Ownership scoping is not optional** (CLAUDE.md backend trap #5): every read and write is
  scoped by the importing user id.

The existing session-based wizard (`import_session.go`) stays the UI surface for CSV/VCF/JSContact
and file-based imports; source imports bypass it by design (a Meerkat DB or Monica snapshot is not
a file upload with per-row merge previews yet). The future assistants (#549/#550) can wrap this
engine or feed a plan's contacts through the records pipeline.

## Consequences

- New packages: `backend/meerkat` (read-only SQLite reader), `backend/monica` (snapshot model +
  loader), `backend/internal/meerkatfixture` (the reviewable-manifest → Meerkat-DB fixture
  loader). New shared framework in `backend/services/import_source.go`; mappers in
  `backend/services/meerkat_import.go` and `monica_import.go`.
- New migration `000045_import_source_links` (a new table — safe against the production-data rule;
  the ledger is derived bookkeeping and its down migration is documented as destroying re-import
  idempotency, not user data).
- The existing `/api/v1/contacts/import/*` endpoints and `ImportSessionManager` are untouched.
- A future Monica v3 importer (tracked as a p3/someday ticket) joins the framework by writing a
  `MapChandlerSnapshot` mapper; no new execution path is needed.
- Mapping tables and known limitations are documented for operators in `docs/import/`.
