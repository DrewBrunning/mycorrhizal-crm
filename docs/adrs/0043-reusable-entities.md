# ADR 0043: Reusable entities — organisations, places, pets

- **Status:** accepted
- **Date:** 2026-10-30
- **Decides:** issue #1999 (ported from fbuchner/meerkat-crm#91). Implementation targets v1.7.0.
- **Related:** `docs/adrs/0001-neutral-hub-and-spoke-contact-model.md`,
  `docs/adrs/0002-correspondence-table-locked-oracle.md`, `docs/adrs/0004-soft-vs-hard-delete-semantics.md`,
  `docs/adrs/0006-revision-token-schema.md`, `docs/adrs/0012-canonical-database-invariants.md`,
  `docs/adrs/0015-temporal-semantics.md`, `docs/adrs/0016-unicode-normalization-and-search-semantics.md`,
  `docs/adrs/0025-temporal-periods.md`, `docs/adrs/0035-service-boundary-and-delete-cascade-registry.md`,
  `docs/adrs/0037-contact-trash-and-restore.md`, `docs/adrs/0038-error-reason-codes.md`,
  ADR 0042 (standing shares, PR #2250, not yet on `main`), backend/AGENTS.md traps #5–#7.
- **Soft dependencies:** #1997 (per-contact field visibility), #1993 (CardDAV ORG/ADR projection), #1998 (standing shares).

## Context

Custom fields are contact-only: `backend/models/field_definition.go#FieldDefinitionTargetContact` is the only
target. A workplace, a house or a pet is re-typed on every contact that shares it, and the relationship graph has
no shared nodes. A contact's ORG and ADR today are plain values inside the neutral Card
(`backend/contactmodel/model.go#Organization`, `backend/contactmodel/model.go#Address`): two colleagues carry two
unrelated copies, and renaming the employer means editing both.

The feature has three hazards, and the decisions below are shaped by them:

1. **Standards fidelity.** ORG and ADR are RFC 9553/6350 properties with locked correspondence rows (`org`,
   `org.unit`, `adr` in `backend/correspondence/testdata/correspondence.tsv`). Anything that changes what a card
   emits touches every CardDAV client.
2. **Production data.** Real contacts already carry ORG/ADR values. Nothing here may rewrite them.
3. **A second graph.** Reusing the relationship graph for entity links would import person-to-person semantics
   (inverse labels, household inference, traversal) into rows they were never designed for.

## Decision

### 1. Entity kinds: a small built-in set

Three kinds, a closed enum: `organisation`, `place`, `pet`. Each kind has a fixed core schema plus custom fields
(decision 5). User-defined entity types are a possible later phase and are **out of scope**; this ADR does not
pre-build for them (no type table, no `type_id`). Widening the enum later is additive.

One table, `entities`, serves all kinds (not three tables), because identity, ownership, sensitivity, delete, merge
and the link table are identical across kinds:

| Column | Meaning |
|---|---|
| `id` | UUID PK, generated in `BeforeCreate` (the `FieldDefinition`/`Household` template); explicit `DeletedAt gorm.DeletedAt` (ADR 0004) |
| `user_id` | owner; every query is scoped by it (trap #5) |
| `kind` | `organisation` \| `place` \| `pet`; immutable after create (a kind change is a delete and re-create) |
| `name` | display name, NFC-normalized on write (ADR 0016), required, at most 200 characters |
| `name_key` | derived normalized key for suggestions and search (decision 9); never client-supplied |
| `core` | JSON, kind-specific fixed schema validated by a typed Go struct per kind; encrypted at rest with the same serializer as `contacts.card` |
| `sensitivity` | `normal` \| `private` \| `secret`, default `normal` (decision 4) |
| revision, `created_at`, `updated_at` | the standard revision token (ADR 0006) so `If-Match` works (ADR 0008) |

Core schemas, deliberately reusing neutral types so projection is a field copy and not a mapping:

- **organisation:** a `contactmodel.Organization` (name, ordered `units`, `sortAs`) plus `url`, `notes`. `name`
  mirrors `Organization.Name`.
- **place:** a `contactmodel.Address` (components, `countryCode`, `coordinates`, `timeZone`, `full`, `contexts`) plus
  `notes`. `name` is the user's label ("Cabin", "Mum's house"), distinct from the postal components. The place's
  `Address.Sensitivity` is *not* stored in `core`; the entity column is the single source (decision 4).
- **pet:** `species`, `breed`, `birth` and `death` as `contactmodel.PartialDate` (ADR 0015), `notes`. Pets never
  project (decision 3).

Rejected: **three tables.** They triple the cascade, merge, link and API surface for no difference in lifecycle.
Rejected: **free-form `core` map.** Unvalidated JSON is the trap `FieldConstraints` was written to avoid; each kind
gets a typed struct with a `validate` tag set.

### 2. Links: a new `contact_entity_links` table, not the relationship graph

A contact is attached to an entity by a **link row**, not an edge.

| Column | Meaning |
|---|---|
| `id` | UUID PK |
| `user_id` | owner (trap #5); the link, the contact and the entity must all share it |
| `contact_vcard_uid` | `Contact.VCardUID` (the graph-entity invariant `FieldValue.EntityID` already follows) |
| `entity_id` | `entities.id` |
| `kind` | copy of the entity's kind, immutable; exists so the primary-per-kind index is expressible |
| `role` | closed vocabulary per kind, see below; free-text `role_label` carries a custom qualifier |
| `is_primary` | at most one live primary per contact per kind (partial unique index) |
| `start`, `end` | `contactmodel.TemporalRange` (`PartialDate` endpoints, ADR 0015/0025); both optional, an all-empty range stores NULL |
| `sensitivity` | `normal` \| `private` \| `secret`, default `normal` (decision 4) |
| `deleted_at`, `deletion_batch_id` | soft delete and the ADR 0037 batch marker (decision 6) |

Roles are a closed set per kind so web, Android and merge agree: organisation `employee|owner|contractor|volunteer|member|other`;
place `resident|owner|visitor|other`; pet `owner|carer|other`. A free-text qualifier belongs in `role_label`
(at most 100 characters), which never participates in matching.

**Uniqueness.** There is no natural-key unique index (a contact can be re-hired by the same organisation: two links,
two periods). The only uniqueness invariants are the primary index, `UNIQUE (user_id, contact_vcard_uid, kind)
WHERE is_primary = 1 AND deleted_at IS NULL`, and a same-owner check enforced in the service (a link whose contact
or entity belongs to another user is rejected 404, never 403, matching ownership behaviour elsewhere). Setting a
new primary demotes the previous one in the same transaction. The first link of a kind for a contact is *not*
automatically primary: primary is a projection switch (decision 3) and must always be a deliberate act.

#### Why not the relationship graph

`relationship-semantics-change` is the checklist for touching the graph, and every item on it is a reason to stay
out. `models.RelationshipEdge` is a person-to-person edge: both endpoints are `Contact.VCardUID`, the `Type` is a
person relation with a derived inverse (`parent_of` ⇄ `child_of`), the cascade deletes an edge from *either*
endpoint (`services.ContactCascadeRegistry`, step `relationship_edges`), confirmed-status edges feed household
inference and the `TraverseGraph` recursive CTE, and `projectRelationshipEdges` synthesizes them into
`Card.RelatedTo`. An entity endpoint would force a typed-endpoint column on every one of those, give "employee of"
an inverse label nobody wants, make traversal walk through a company to reach every colleague, and let household
inference see a shared office as a household. Keeping links in their own table leaves person-to-person semantics
byte-for-byte untouched, and requires no change to the relationship type vocabulary or its fixtures.

#### Graph view: read-only rendering

The graph endpoint (`backend/controllers/graph_controller.go#GetGraph`) keeps its current node and edge types. When
a client opts in with `?include_entities=true` it additionally returns `entity_nodes` and `entity_edges` arrays
(separate keys, never mixed into `nodes`/`edges`), built from live links of normal sensitivity, scoped by the same
`full`/`from`/`depth` rules: an entity node appears only if at least one in-scope contact links to it, and `depth`
counts a contact→entity hop as one. Entity edges are display-only: they carry no type, no inverse, no status and are
never fed to traversal, household inference or the suggestion engine. The web graph and the Android graph render
them with a distinct node style and no edit affordances; editing happens on the contact or entity page. The default
response is unchanged, so no existing client sees a difference. Entity nodes count against the web render ceiling
(`frontend/src/utils/graphBudget.ts`) like any other node.

### 3. Projection into the neutral Card, vCard and CardDAV

**Only the link marked primary projects, and only into ORG (organisation) or ADR (place).** Pets and non-primary
links never project. A link with `end` in the past, or `start` in the future, is not *current* and does not
project even if flagged primary; the flag is retained so a past employer that is still marked primary reappears
if its end date is cleared. "Now" comes from the injected clock and the owner's calendar day (ADR 0015); endpoints
that cannot be compared to today at shared precision count as current (ADR 0015 rule 3: non-comparability is not an
error).

**Projection is read-time synthesis, never written back to `Contact.Card`.** This is the exact shape of
`backend/models/contact_record.go#projectRelationshipEdges`, `projectTags` and `projectCustomFields`, and it is
deliberate: the stored Card stays the contact's own data, the flat columns and `DeriveProjection` keep reading it
only, and deleting a link or entity makes the projected value vanish with no write to the contact. A new
`projectPrimaryEntities` step sits beside those calls in `RecordForContactFiltered`, so it reaches REST detail,
CardDAV/WebDAV (`RecordForContactForSync`), VCF/JSContact export and contact shares by the same path and is
filtered by the same `FieldSelection`.

**Precedence (the contact's own value and a primary link both exist).** The contact's own values are never replaced,
reordered behind a projection, or overwritten. The projected entry is *appended* after the contact's own entries
with a `pref` strictly greater than any own entry's, unless it is value-equal to an own entry after ADR 0016
normalization (then it is dropped as a duplicate). Consequences, stated so they are not surprises:

- A contact with no own ORG/ADR simply shows the entity's value (the common case).
- A contact with its own ORG and a primary organisation link emits two ORG entries on vCard 4.0 and JSContact. On
  vCard 3.0 only the first ORG is emitted (existing adapter behaviour), so the projection is dropped from that
  serialization only and a warn `Diagnostic` names it (ADR 0002 tier 2). The own value still leads.
- `contacts.organization` and the other flat columns, the contact list, search and sort are unchanged: they derive
  from the stored Card. An organisation-only contact therefore shows its entity in the link UI and under
  `GET /contacts?entity_id=`, not in the legacy `organization` column. Making list and search read through the
  projection is a deferred decision (Open questions).

**Adopt.** `POST /contacts/{id}/entity-links/{linkId}/adopt` copies the entity's current org/address into the
contact's own Card as an ordinary contact value (through `ApplyRecordToContact` and the card-merge path; never by
setting `Contact.Card` directly), then leaves the link in place. The copy takes a fresh element ID, so it is not a
projection echo. It is an explicit user action and is audited like any contact edit. A subsequent projection of the
same link is value-equal and is suppressed, so no duplicate appears. After adopt the entity and the contact value
diverge independently; that is the point (a snapshot the user chose to keep). A reverse action, **link from
existing**, takes one of the contact's own ORG/ADR values and either matches it to a suggested existing entity or
creates a new one from it, then creates a link; it does not remove the contact's own value. Both are separate,
explicit endpoints; neither is ever triggered by an import, a sync or a merge.

**Echo suppression (the write-side pair).** Because projected entries appear in the card a client downloads, a
client that PUTs the card back would otherwise write them into the contact. The projected entry therefore carries a
reserved element ID, `ent-<linkId>`: in vCard 4.0 it is a valid `PROP-ID`, in JSContact a map key, so well-behaved
clients round-trip it. The inbound path (`ApplyRecordToContact` as reached from CardDAV PUT, WebDAV, VCF import and
REST nested writes) drops any ORG/ADR entry whose ID has the `ent-` prefix. For clients that strip IDs (vCard 3.0,
and several native address-book apps), it additionally drops an incoming entry that is value-equal to the
*currently projected* entry **and** not value-equal to any value the contact already stores. A user who edits a
projected value in such a client therefore creates an ordinary contact value (no loss, an explainable duplicate the
UI can resolve with adopt), and an unchanged echo never accumulates. This mirrors the
`models.PreserveSensitiveAddresses` / `models.CarryForwardAddressSensitivity` pair for the opposite hazard, and
needs the same dedicated tests: PUT of an unchanged projected card is a no-op on the stored Card.

**Inbound ORG/ADR from CardDAV, vCard and JSContact import.** Recommendation, adopted: **inbound values stay
contact values.** Import and sync never create or match an entity. The reasons: (a) "nothing merges automatically"
(decision 8) applies equally to automatic creation, because a wrong automatic match between "Springfield Elementary"
and "Springfield Elementary School" attaches a person to the wrong shared record, and unwinding it needs the merge
tool; (b) CardDAV sync is a bulk, repeated path where an auto-match would run on every pull and make a remote client
able to mutate shared records; (c) it is the only choice with no new loss path, since the value lands where it lands
today. The user reaches entities from imported data through the suggestions helper (decision 9), which is the
deliberate act.

**Correspondence and fidelity.** No row is added to or changed in the locked correspondence table (ADR 0002):
projection lands in the existing `org`, `org.unit` and `adr` rows, and the reserved ID is an ordinary use of the
`id` field every collection already carries (ADR 0001). The entity, its core fields beyond ORG/ADR (url, notes, pet
data), its custom fields and the link role, dates and primary flag have **no standards home** and are CRM-internal,
like circles and ADR 0025 periods: they are never serialized into a vCard or JSContact. They are carried only in the
account bundle (decision 7), and a vCard export that omits the role/dates is the same named, documented loss ADR
0025 already records for periods. `TemporalRange` is not emitted as an invented `X-` property. Fidelity gates to run
at implementation (contact-data-fidelity skill): the correspondence and adapter suites, the golden fixtures
unchanged, a new round-trip test (project → export → import → stored Card unchanged), and
`go run ./cmd/gencompatmatrix` showing no diff.

### 4. Ownership, sensitivity and sharing

**Per-user, never shared.** Every entity and link row has a `user_id`; every query filters by it, and a link is
resolved by `id AND user_id`. There is no cross-user entity: two users who both know "Acme" each have their own.

**Standing shares (ADR 0042, #1998).** Entities are **not a v1 share scope** (ADR 0042 decision 4 lists contact,
note, activity and circle). Consequences, which the ADR 0042 field census and review should treat as the rule: a
contact grant's JSContact view is produced by `RecordForContactFiltered` with `IncludeSensitive` forced false and
the granted `sections`, so a *primary* link's projected ORG/ADR appears inside the `organizations` / `addresses`
sections exactly like an own value and is filtered by the same query-level sensitivity rule; entity ids, link rows,
roles and dates never appear, because they are not in the neutral Card. The one-time `ContactShare` path behaves
identically. A recipient can never read an entity's `notes`, custom fields, pet data or other links.

**Sensitivity.** Both the entity and the link carry `normal|private|secret`. A projected entry's effective
sensitivity is the higher of the two. It is applied in the query, before synthesis:
`projectPrimaryEntities` takes the same `includeSensitive` argument as its siblings, and with it false selects only
links *and* entities at `normal`, so a private place is not projected into a synced card at all. For a place, the
projected `Address.Sensitivity` is additionally stamped with the effective value, so `models.ApplyFieldSelection`
and `RecordForContactForSync` (which already drop above-normal addresses with their `geo:` coordinate) and the
geocoder gate (ADR 0031: an address above normal is never sent to the geocoder without opt-in) behave identically for
a projected place and an own address, with no new code path to forget. Organisations have no address-style carrier
for the token, so the query filter is their only guard, and the stamping is a defence-in-depth for places. The owner's
own surfaces keep full fidelity, matching the documented exceptions in CLAUDE.md: REST detail and `/entities`
(sensitivity is shown, not hidden), the flat CSV backup, the account bundle and the contact map. The entity list
does not hide `secret` rows from their owner. Unlike `ContactShare`, nothing about an entity leaves the instance
except a projected ORG/ADR value, and that is filtered.

**Field visibility (#1997) is cosmetic and does not apply to entity fields.** #1997's per-contact visibility
toggles which custom fields a contact page shows. It is a UI preference, not an access control, and it is keyed by
(contact, definition). Entity custom fields are shown on the entity page, always, and #1997 has no effect on them
unless it is later extended by its own decision. It never gates what an export, sync or share emits (sensitivity does).

**MCP (ADR 0032).** Entities are not exposed by any MCP tool in v1. The MCP `get_contact` tool reads through
`RecordForContactFiltered` and so reflects projected ORG/ADR at the default (non-sensitive) setting; nothing else
changes.

### 5. Fields: reuse `FieldDefinition` with new targets

Reuse, do not fork. `FieldDefinition.Target` widens from `contact` to `contact|organisation|place|pet`
(`backend/models/field_definition.go#FieldDefinition`, validator `oneof`, and the closed constants beside
`FieldDefinitionTargetContact`). All ten value types, `FieldConstraints`, `Position`, `Group` and `Sensitivity` carry
over unchanged, as does `services.ValidateFieldValue`, so values are typed and validated by the same code that
protects contacts. A second schema would duplicate that validator, the Settings editor, the web field editor and the
Android field editor.

Rules that differ for entity targets:

- **`Projection` must be `internal-only`** for any non-contact target. `vcard:X-<NAME>` projection is a contact
  concept (`projectCustomFields` reads `FieldValue` by contact UID); the definition validator rejects a projection on
  an entity target 422. This also guarantees no entity custom field can reach a card, which keeps decision 3's
  "only ORG/ADR project" true.
- **Key uniqueness becomes per (user, target, key).** Today `idx_field_definition_user_key` is `(user_id, key)`; the
  migration replaces it with `(user_id, target, key)` so "notes" can exist on both a contact and an organisation.
  The change only *relaxes* the constraint, so no existing row can violate it, and no backfill is needed (existing rows
  are all `contact`).
- **`FieldValue.EntityID` already means "the id of the thing the value belongs to"** and is `uuid4`-validated, which
  an `entities.id` satisfies. It is overloaded across contact UIDs and entity ids; the definition's `Target`
  disambiguates, and the data-integrity checker (ADR 0012) gains one invariant: a `FieldValue`'s `entity_id` must
  resolve to a live row of the kind its definition targets. The `field_values` cascade step is keyed by contact UID
  and so cannot reach an entity's values; the entity delete adds its own `field_values` step (decision 6).
- **Sensitivity** on a definition behaves as for contacts; an entity field above normal is hidden from any
  non-owner surface (there are none in v1) and shown to the owner.

### 6. Delete, cascade and trash

**Soft vs hard (ADR 0004), decided once per model.**

- `entities` — **soft delete** (user-authored content, gives undo and a tombstone).
- `contact_entity_links` — **soft delete**. ADR 0004 hard-deletes join rows because a dead row would block a
  natural-key unique index; this link has *no* natural-key unique index (re-hire is two links) and its only unique
  index (primary) is partial on `deleted_at IS NULL`, so the stated reason does not apply, while the link carries
  authored data (role, dates, primary) that trash restore must be able to bring back. This is a deliberate,
  recorded exception to the join-row rule, in the same family as "an entity that must soft-delete and carry a
  unique key makes the index partial".
- Custom `field_values` of an entity — **hard** (join-shaped, as for contacts); an entity restore therefore needs
  them from a snapshot (below).

**Entity delete cascades to its links.** `DELETE /entities/{id}` runs in one transaction: it soft-deletes the entity,
soft-deletes its live links stamping one new `deletion_batch_id`, and hard-deletes its `field_values`, writing the
removed field values into a small `entity_deletion_snapshots` row (`payload` JSON, `payload_version`, encrypted with
the same serializer, deleted on purge) — the ADR 0037 mechanism at entity scale, sharing its batch-marker rule
(decision 2) and its "snapshot exists iff the delete committed" transaction rule. Purge (`PurgeSoftDeletedRows`)
hard-deletes the entity, its links and its snapshot together past `DELETED_RETENTION_DAYS`; the user-delete cascade
(`deleteUserCascade`) removes all three tables. A per-entity delete cascade list is registered in a new declarative
`EntityCascadeRegistry` (same `CascadeStep` shape as the contact registry, ADR 0035), with a coverage test failing
when a table that references `entities.id` has no step. Adding an entity-referencing table without a step is the
silent failure ADR 0035 exists to prevent.

**Contact delete removes its links via `ContactCascadeRegistry`.** One new step, `contact_entity_links`, `Mode:
CascadeSoft`, matched `byContactUID`, so the links are soft-deleted with the contact's batch id and ADR 0037 restore
revives them by `deletion_batch_id` with no payload. The entity itself is untouched; an organisation with no remaining
links stays (the user may want an empty organisation). Because the link carries `is_primary`, a restored link that
would collide with a primary created in the meantime is revived with `is_primary = false` and reported in the restore
response, the same "restore reports, never fails silently" pattern ADR 0037 uses for dangling edges. A link whose
*entity* was deleted in the meantime is skipped and reported.

**Entity restore.** `POST /entities/{id}/restore` (the contact-trash pattern, ADR 0037) revives the entity, revives
links whose `deletion_batch_id` matches, re-creates field values from the snapshot, and re-checks the primary index
(demoting on collision, reported). A live entity with the same `name_key` is not a conflict (names are not unique,
decision 9); the response warns instead.

**What happens to a projection when its entity is deleted.** Recommendation, adopted: **the contact keeps no ORG/ADR
from it.** The link is soft-deleted, so the read-time projection simply stops; the contact's own values, if any,
were never touched. We do **not** fall back to a copy, because a copy would require writing the entity's value into
the contact at delete time, which is exactly the silent write the maintainer ruled out. A user who wants to keep the
value uses **adopt** before deleting, and the delete confirmation says how many contacts currently receive a
projection from this entity ("Acme is the primary organisation of 14 contacts; they will lose that organisation")
with an adopt-all affordance as a UI convenience. Restoring the entity brings the projection back.

### 7. Import and export

- **Account bundle: full fidelity.** `models.AccountBundlePlan` gains `entities`, `entity_links` and
  `entity_field_values` (and the entity-target `field_definitions` it already exports), including sensitivity and
  soft-deleted state only as far as existing bundle rules already carry it (live rows only). Cross-references use the
  stable ids (`entities.id`, `contact_vcard_uid`), never uint PKs, like every other bundle collection. The bundle
  version stays `1`: the new collections are additive and optional on read (absent means none), and the importer
  ignores no field. `omitted` is unchanged. Restore remaps nothing because ids are UUIDs, but a collision with an
  existing entity id is a 409-style conflict reported per row, not an overwrite. The account-bundle coverage test
  (`backend/controllers/account_bundle_coverage_test.go`) must list the new tables, failing otherwise.
- **CSV export: full-fidelity exception.** `ExportData` appends two sections after the existing ones, in the same
  `=== NAME ===` format and passing the same `csvSafe` formula-injection neutralization: `=== ENTITIES ===` with
  `ID, Kind, Name, Sensitivity, Core (JSON), Notes, Created At, Updated At`, and `=== ENTITY LINKS ===` with `ID,
  Contact ID, Contact Name, Entity ID, Entity Name, Kind, Role, Role Label, Primary, Start Date, End Date,
  Sensitivity`. Entity custom-field values are appended as a third section `=== ENTITY FIELDS ===` (`Entity ID,
  Definition Key, Value`). Nothing is filtered, matching the relationship section (it is the owner's backup). The CSV
  *contact* section is unchanged: it is not extended with a projected organisation column, so an import of a
  pre-1.7 CSV is unaffected and the CSV round-trip of the contact section is lossless as before.
- **CardDAV: projection only.** No `VCARD`-like entity object is exposed, no new collection, no `X-` property. A
  CardDAV client sees entities only as the projected ORG/ADR values of decision 3, with the `ent-` id.
- **vCard/JSContact import:** inbound ORG/ADR stay contact values (decision 3). **Monica and Meerkat imports:** no
  mapping. Neither source models a reusable entity that the neutral model could represent; Monica's company is a
  string that already lands as a contact organization (`backend/services/monica_import.go`), and stays a contact value
  (the `source-import` framework, ADR 0007, gains no entity kind, and `ImportSourceLink` gains no `entity` kind). The
  suggestions helper (decision 9) is how a user turns those imported strings into entities afterwards.

### 8. Dedupe: manual merge with suggestions; nothing automatic

**Merge** is `POST /entities/{id}/merge` with `{ "into": "<survivor id>" }`, owner-scoped, same `kind` required (422
otherwise), the entity itself not allowed as its own survivor. In one transaction:

1. links of the loser are **re-pointed** to the survivor (`entity_id` update). If a contact then has two links to the
   survivor with identical `role`, `start` and `end`, the later duplicate is soft-deleted (it asserts nothing the
   other doesn't). If re-pointing would give a contact two primaries of the kind, the survivor's existing primary
   keeps the flag and the re-pointed one is demoted, reported in the response;
2. the survivor keeps its own `core`, `name` and custom-field values. Fields the survivor leaves empty are filled
   from the loser only where the request lists them in `resolutions` (the shape and reject-not-merge posture of
   `services.ContactMergeResolution`; field-by-field, explicit, no silent union). Custom `field_values` are re-keyed
   to the survivor when the survivor has no value for that definition, otherwise the survivor's wins and the loser's
   is reported;
3. the loser is **soft-deleted** with a batch id and a deletion snapshot, so it appears in trash and can be restored
   (restoring a merge loser revives the entity only; re-pointed links stay with the survivor and the response says
   so, matching ADR 0037 treatment of a merge-loser contact);
4. the merge bumps the revision of the survivor and every touched link (ADR 0006/0008), and is audited.

Because projection is read-time, the survivor's value reaches every re-pointed primary link's contact on the next
read with no per-contact rewrite, and **adopted copies are not touched**.

**Suggestions** are `GET /entities/suggestions?kind=...`, returning candidate pairs of live entities of the same
kind plus, for the "create from my contacts" helper (decision 10), candidate groups of contact values. Matching uses
the ADR 0016 normalization and nothing weaker:

- `name_key` = NFC (already stored), Unicode lower-casing, runs of whitespace collapsed, leading/trailing
  punctuation trimmed; `EqualFold` on this key, the same fold sort names use. The suggestion tier additionally drops
  a short closed list of legal suffixes (`inc`, `llc`, `ltd`, `gmbh`, `ag`, `corp`, `co`, `sa`, `bv`) *for
  comparison only* — it is never written back and never used for the exact tier. Diacritic folding is **not** added:
  ADR 0016 explicitly leaves it to the search tokenizer, not the data layer.
- for places the key is the normalized joined components in `Address` order (`number`, `name`, `locality`,
  `postcode`, `country`), punctuation and whitespace collapsed, so "12 High St." and "12 High St" match.
- pets match on `name_key` plus `species` only.
- results carry a `reason` (`exact_name`, `normalized_name`, `address_components`) and are sorted for stable paging.
  A dismissal table (`dismissed_entity_pairs`, the `DismissedDuplicatePair` pattern, ordered id pair, hard-deleted,
  covered by the entity cascade) lets the user hide a pair for good.

Nothing merges, links or creates from a suggestion without a user request. There is no background job and no import
hook; the suggestions endpoint is read-only and idempotent.

### 9. Migration

Existing ORG/ADR values **stay contact values**. No migration rewrites a Card, no startup job converts, and the
upgrade (one schema migration: `entities`, `contact_entity_links`, `entity_deletion_snapshots`,
`dismissed_entity_pairs`, the widened `field_definitions` index) is purely additive; the `.down.sql` drops the new
tables and restores the old index only if no non-contact definition exists, otherwise it refuses with a clear
message, so a downgrade never silently deletes entity data (database-migration skill: preserve data, symmetric down,
upgrade-floor tests, schema dump regenerated via `genschema`). A new migration needs a `pii-inventory.md` entry (the
`docscheck` gate checks every migrated table appears there) and a data-retention row.

**Optional helper, user-triggered:** "Create entities from my contacts' organisations / addresses" in Settings. It
calls the suggestions endpoint in `contact_values` mode, shows groups ("Acme Corp — 7 contacts", "Acme, Inc — 2
contacts") with per-group checkboxes, and on confirm creates one entity per group and a link per contact (marked
primary only where the contact has no other primary of that kind and the user ticked "set as primary"). It never
deletes or edits a contact value (the value stays, and the projection is suppressed as a duplicate). It is
resumable (re-running proposes only unmatched values) and is the only bulk path.

### 10. API and UI outline

All under `/api/v1`, owner-scoped, `If-Match` on writes (ADR 0008), `Idempotency-Key` on creates (ADR 0010), errors
with `reason_code` (ADR 0038). Each is an `api-contract-change` and must be added to the route-matrix stale-write
tests (ADR 0009).

| Route | Purpose |
|---|---|
| `GET /entities?kind=&q=&sensitivity=&cursor=` | list; `q` is folded with `services.NormalizeSearchTerm` |
| `POST /entities` | create (`kind` required; validates `core` for the kind) |
| `GET/PUT/DELETE /entities/{id}` | read, update (explicit allowlist, no mass assignment), delete with the cascade of decision 6 |
| `POST /entities/{id}/restore` | restore from the deletion snapshot |
| `POST /entities/{id}/merge` | merge (decision 8) |
| `GET /entities/suggestions` | dedupe and contact-value suggestions |
| `POST /entities/suggestions/dismiss` | hide a pair |
| `GET /entities/{id}/contacts` | linked contacts (live links) with role, dates, primary |
| `GET /contacts/{id}/entity-links` | a contact's links, each with the projection preview ("projects as ORG: Acme") |
| `POST /contacts/{id}/entity-links` | create a link |
| `PUT/DELETE /contacts/{id}/entity-links/{linkId}` | edit role/dates/primary/sensitivity; delete (soft) |
| `POST /contacts/{id}/entity-links/{linkId}/adopt` | decision 3, adopt the entity's value into the contact |
| `POST /contacts/{id}/entity-links/from-existing` | decision 3, link from one of the contact's own ORG/ADR values |
| `GET /contacts?entity_id=` | filter contacts by linked entity |

`POST /field-definitions` accepts `target` of the new kinds; the field-value routes accept an entity id for a
definition of that kind (they already key by `entity_id`).

**Web (sketch).** An "Entities" nav item with kind tabs and a list; an entity page with the core form, custom
fields, and a "People" list (roles, periods, primary); on the contact page, a new "Organisations, places & pets"
section (needs both a `SectionGroup` id *and* a `ContactJumpNav` `sections[]` entry, per the recorded gotcha) with
add/edit link, primary toggle, and the adopt / link-from-existing actions; a merge dialog reached from suggestions; a
Settings "Create entities from my contacts" wizard; the graph entity-node style (decision 2). **Android:** the same
contact section and entity list/detail with the existing `DateField` period inputs (ADR 0025), the same adopt and
merge flows, and the graph opt-in; delivered as its own tier, backend-first. All UI strings go through the
localization skill; the contact page never shows a projected value without a "from <entity>" chip, so users can tell it
from their own.

## Consequences

- The relationship graph, its fixtures, inverse labels, traversal and household inference are unchanged.
- The neutral Card and the correspondence table are unchanged; standards clients see two new, ordinary ORG/ADR
  entries for contacts that opt in by marking a primary link, and nothing else.
- A new cascade registry, a new snapshot table and one more `FieldDefinition` target broaden the surface that tests
  must cover: ownership regression tests per route (trap #5), a registry-coverage test for `EntityCascadeRegistry`,
  echo-suppression tests, sensitivity-leak tests per leaving surface, and the account-bundle coverage test.
- An adopted copy and the entity diverge independently; this is documented behaviour, not a sync bug.
- Contact list, search and sort do not see an organisation-only contact's entity (deferred, below).
- A vCard 3.0 client sees only the first ORG, so a contact with its own ORG will not show its projected organisation
  there (named diagnostic).
- New persistent user data requires `docs/security/pii-inventory.md`, `docs/security/data-retention-lifecycle.md`
  and `docs/security/asvs-l2.md` updates in the implementation PRs, plus a threat-model note that entity ids are never
  exposed on a leaving surface.

## Rejected alternatives

- **Using the relationship graph** (decision 2).
- **A separate field schema for entities** (decision 5): duplicates validation, editors and the visibility concept.
- **Writing the entity's value into the contact on link or on primary change:** overwrites or duplicates the
  contact's data, the thing the maintainer ruled out, and makes entity edits fan out to N contact writes with ETag
  bumps and CardDAV churn.
- **Automatic entity creation or matching on import or sync** (decision 3).
- **Hard-deleting links:** loses role/date/primary data that trash restore should return (decision 6).
- **Merging automatically** on exact name match: even exact matches are different employers sometimes ("Springfield
  Elementary" in two states).

## Open questions for the maintainer

These are real trade-offs the recommendation above settles provisionally; each can be reversed before
implementation without invalidating the rest.

1. **Own-value precedence: append vs. suppress.** Recommended: append after own values (vCard 4.0 and JSContact show
   both; vCard 3.0 shows only the own one). The alternative suppresses a primary link whenever the contact has any
   own ORG (or, for ADR, any own address), which avoids the v3 loss and duplicate-looking entries but hides a place
   link behind an unrelated home address, which defeats the feature for places. Reverse if mixed ORG lists confuse
   downstream clients in practice.
2. **Past/future links stop projecting.** Recommended (current-only). Alternative: the primary flag alone decides.
   The risk of the alternative is a stale ex-employer on every card until the user remembers to move the flag.
3. **Should list, search and sort read through the projection?** Recommended: not in v1 (the flat columns stay
   derived from the stored Card, ADR 0002 appendix). The cost is that an organisation-only contact is not found by
   typing the company name into contact search. A later change could add the primary organisation name to the FTS
   document and `sort_name` derivation, which is an ADR 0016/0012 INV-D9 change in its own right.
4. **Entity restore scope.** Recommended: restore reuses the ADR 0037 snapshot pattern at entity scale (a fourth
   table). A lighter v1 would omit entity restore and rely on purge-window recovery by support, accepting that an
   accidental organisation delete loses all its links' roles and dates. The snapshot is cheap given the pattern, but
   it is a second restore path to maintain.
5. **Entities in MCP and standing shares.** Recommended: neither in v1. Revisit after #1998 ships if users ask to
   share "the family cabin" as an object rather than through the contacts that live there.
6. **Pet projection.** There is no standard ORG/ADR equivalent for pets; they are CRM-internal. If the maintainer
   wants pets in vCard, the only honest route is `KIND:individual` with `X-` properties, which ADR 0002 forbids
   inventing without a spec home.
7. **Legal-suffix list and diacritic folding for suggestions** are English-biased by design. A locale-aware list is
   a localization follow-up; the exact tier is unaffected.
