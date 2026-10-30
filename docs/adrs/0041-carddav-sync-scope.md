# ADR 0041: CardDAV sync scope by circle

- **Status:** accepted
- **Date:** 2026-10-30
- **Decides:** issue #1993 (the ADR its acceptance list requires before implementation).
- **Related:** `docs/adrs/0004-soft-vs-hard-delete-semantics.md`, `docs/adrs/0006-revision-token-schema.md`,
  `docs/adrs/0035-service-boundary-and-delete-cascade-registry.md`, `docs/adrs/0036-app-password-scopes.md`,
  `docs/adrs/0037-contact-trash-and-restore.md`, `docs/adrs/0038-error-reason-codes.md`,
  `docs/adrs/0039-circle-metadata-and-aggregate-timeline.md`, backend/AGENTS.md traps #5-#7 and the
  sensitivity rule.

## Context

The CardDAV server exposes exactly one address book per user (`backend/carddav/backend.go#Backend.ListAddressBooks`)
and serves every live contact in it (`backend/carddav/backend.go#Backend.ListAddressObjects` filters by `user_id`
only). A user with a large historical contact set cannot sync just the circles they need to a phone.

Facts about the current server that the design depends on, verified in code:

- **Contacts are in a circle through `circle_members`**, keyed by `member_vcard_uid`
  (`backend/models/circle.go#CircleMember`). The flat `Contact.Circles` JSON column is a legacy field that no UI
  surface reads; it is not the scope key.
- **Every read path funnels through two functions.** `QueryAddressObjects` delegates to `ListAddressObjects`, and
  go-webdav's addressbook-multiget, resource PROPFIND, HEAD and GET all call `GetAddressObject` per href, turning an
  error into a per-href response (go-webdav v0.7.0, `carddav/server.go`). Scope can therefore be enforced at two
  chokepoints.
- **There is no `sync-token`, no `getctag`, and no sync-collection REPORT.** go-webdav does not implement them
  (`docs/carddav.md`, "Interoperability limitations"). Clients PROPFIND depth 1, compare ETags with their local copy,
  and delete locally whatever the server no longer lists. A contact's ETag is derived from its own revision
  (`backend/models/contact.go#Contact`, `ETag`). ADR 0037 decision 7 says the server "reports soft-deleted contacts as
  removed in its sync report"; precisely, a soft-deleted contact simply stops appearing in the listing. This ADR uses
  that wording.
- **Sensitivity is a content filter, not a contact filter.** `contactToAddressObject` builds the card with
  `backend/models/contact_record.go#RecordForContactForSync`, which withholds above-normal addresses, personal-info
  entries and graph rows from the card; `PutAddressObject` re-attaches what the client never saw. A contact is never
  hidden from CardDAV for being sensitive.
- **Circles hard-delete** (no `deleted_at`), and `circle_members.circle_id` is `ON DELETE CASCADE` with
  `foreign_keys(1)` enabled (`backend/database/migrate.go`). `backend/controllers/circle_controller.go#DeleteCircle`
  relies on that; `backend/controllers/user_delete_cascade.go#deleteUserCascade` deletes members then circles by hand.
- **`DeleteAddressObject` calls a bare `db.Delete(&contact)`** and does not use `services.DeleteContact`, so a CardDAV
  delete runs no cascade-registry step and writes no trash snapshot (ADR 0035, ADR 0037). That is a pre-existing gap
  tracked and fixed separately in #2249, which lands before this work (decision 5).

## Decision

### 1. One filtered address book

The single address book keeps its path, `/carddav/addressbooks/{username}/contacts/`, its name, and its advertised
vCard versions. Discovery (`/.well-known/carddav`, the principal, the home set, `ListAddressBooks`,
`GetAddressBook`) is unchanged. There is **no** address book per circle: a contact in two circles would appear twice
under one UID across books, and moving a contact between circles would be a delete plus a create on the phone.

Each user has a **sync scope** with one of three modes:

| Mode | A contact is in scope when |
|---|---|
| `all` (default) | always. No predicate is added; behaviour is identical to today. |
| `include` | it is a member of at least one included circle. |
| `exclude` | it is a member of **none** of the excluded circles (so a contact with no circles is in scope). |

Membership is `circle_members` only. A user with no scope row is in mode `all`; installs upgrade with no behaviour
change and no backfill.

### 2. Storage

Two new tables (hand-written migration pair at the next free number plus its `.down.sql`; `database-migration`
skill):

- `carddav_sync_scopes`: `user_id` (PK, FK to `users`), `mode` (`CHECK` in `all`, `include`, `exclude`),
  `default_circle_id` (nullable, FK to `circles(id)` **`ON DELETE SET NULL`**), `created_at`, `updated_at`.
- `carddav_sync_scope_circles`: `user_id`, `circle_id` (FK to `circles(id)` **`ON DELETE CASCADE`**), primary key
  `(user_id, circle_id)`. One table serves both modes; the mode says whether the rows include or exclude.

Circles are stored by their stable UUID, so a rename never touches the scope. A join table rather than a JSON column
gives referential integrity and lets a circle delete drop the entry in the same statement that deletes the circle,
including on code paths that bypass `DeleteCircle`.

**Ownership.** The write handler resolves every submitted circle id with `id = ? AND user_id = ?` and rejects the
whole request with `400` (a reason code per ADR 0038) if any id is not the caller's; an unknown id and another user's
id are indistinguishable. The scope predicate re-checks ownership at read time by joining `circles` on `user_id`, so a
row written by any future path still cannot name another user's circle (trap #5).

**Validation on write** (the whole singleton is replaced by `PUT`; there is no partial update):

- `all`: `circle_ids` must be empty and `default_circle_id` null. Switching to `all` clears the selection; the web UI
  keeps its draft locally if it wants to offer undo.
- `include`: at least one circle; `default_circle_id` is **required** and must be one of the included circles.
- `exclude`: at least one circle (an empty exclusion is just `all`); `default_circle_id` is optional and, when set,
  must **not** be one of the excluded circles.

These are write-time rules only. The states reachable afterwards through circle deletion (decision 3) are a superset
and are handled by derived warnings, not rejected.

**API.** `GET` and `PUT /api/v1/users/me/carddav-sync-scope` (additive, so not a breaking change under
`docs/breaking-change-policy.md`; `api-contract-change` skill: OpenAPI, reason codes, contract tests). The `GET`
response carries `mode`, `circle_ids`, `default_circle_id`, `updated_at`, `in_scope_count`, `total_count` and a
`warnings` array (decision 3). `POST /api/v1/users/me/carddav-sync-scope/preview` takes a proposed scope and returns
the counts without persisting, so the UI can show "N contacts will be removed from your phone" before the user saves.
The `PUT` is last-writer-wins on the caller's own settings, like the other per-user configuration rows (the
`reasonIntegration` class in `backend/routes/conditional_write_matrix_test.go`); it needs no `If-Match`.

**Cascades.** `backend/controllers/user_delete_cascade.go#deleteUserCascade` is hand-written, so the implementing PR
adds a delete of both tables before `circles` and a test (trap #6). Neither table references a contact, so
`ContactCascadeRegistry` and the ADR 0037 snapshot need no step: deleting or restoring a contact never touches the
scope. Both tables hold only ids and a mode, no PII; `docs/security/pii-inventory.md` and
`docs/security/data-retention-lifecycle.md` get rows in the implementing PR. Both are hard-delete, edge-shaped rows
(trap #7).

**Account bundle.** The scope is deliberately **not** part of the account bundle (`GET /api/v1/export/account`), an
exception to the bundle's full-fidelity rule: it is per-deployment device configuration and circle ids may be remapped on
restore. After a bundle import the scope is `all`.

### 3. Deleting a circle that is in the scope

The circle is **dropped from the scope** (the join row cascades), and `default_circle_id` is set null if it pointed at
it. **Server data is never deleted and nothing is auto-widened.**

- An `include` scope that loses its last included circle **stays `include` and empty. It syncs nothing**: the listing
  is empty and a client deletes its local copies on the next sync. It never falls back to `all`.
- An `include` scope that loses its default circle but keeps others stays valid for reads. It now has no default
  circle, so a client-created contact cannot be placed (decision 5, case C) until the user chooses a new default. The
  server never picks one: silently filing new contacts into an arbitrary circle is a hidden write.
- An `exclude` scope that loses a circle shrinks its exclusion list; with an empty list it is equivalent to `all`. This
  follows from the circle's members no longer being in any circle; no rule beyond the decision is invented here.

**How the user is told (recommendation; the decision above does not depend on it).** A circle delete can come from
the web, Android, the REST API or the account-delete path, and a toast at one call site would miss the others. The
state is therefore **derived on read**: `GET .../carddav-sync-scope` computes `warnings` from the stored rows, so the
signal survives until the user fixes it and cannot be dismissed by accident:

| Code | Condition |
|---|---|
| `scope_empty` | mode `include` with zero circles. Nothing syncs. |
| `default_circle_missing` | mode `include` with circles but no default circle. Phone-created contacts are refused. |
| `exclusion_empty` | mode `exclude` with zero circles. Every contact syncs. |

The web Settings page shows a persistent banner for any warning. Because a cascade cannot prompt, the web
circle-delete confirm dialog also reads the scope first and, when the circle is in it or is the default, adds a line
such as "This circle is part of your CardDAV sync scope; deleting it will leave the scope empty" or, for an excluded
circle, "its contacts will start syncing to your phone". The latter matters: deleting an excluded circle removes the
reason those contacts were hidden. A one-time system notification (`docs/notifications.md`) is a reasonable addition
but needs its own dedupe state and is not required.

### 4. Reads

All read predicates are added **in the query**, never by post-filtering. One helper in `backend/carddav` builds the
predicate from a subquery on the scope tables (not a list of ids fetched first), so reading the scope and the contacts
is one statement and cannot race a concurrent scope edit:

- `include`: `EXISTS (SELECT 1 FROM circle_members cm WHERE cm.member_vcard_uid = contacts.vcard_uid AND cm.user_id =
  contacts.user_id AND cm.circle_id IN (<owned scope circle ids>))`.
- `exclude`: the same with `NOT EXISTS`.
- `all`: no clause.

The predicate is applied by `ListAddressObjects` (and so `QueryAddressObjects`, whose library `Filter` then runs over
the already-scoped set) and by **both** lookups in `GetAddressObject`: the `vcard_uid` lookup and the legacy numeric-id
fallback. Consequently:

- PROPFIND depth 1, addressbook-query and filter-less addressbook-query list only in-scope contacts.
- addressbook-multiget, resource PROPFIND, HEAD and GET for an out-of-scope or unknown href return **404** (multiget: a
  404 status for that href inside the multistatus). A contact is never revealed by being asked for by name.
- `Archived` stays orthogonal: an archived contact is served today and is served or hidden by scope exactly like any
  other.
- A contact created by an inbound CardDAV *subscription* is an ordinary contact and is subject to the scope. It has no
  circle and gets no default circle (that applies to server PUTs only, decision 5), so it stays out of `include`
  scopes. This is as designed: the web settings help and `docs/carddav.md` must say so.

**Sensitivity is independent.** Scope decides *which contacts*; `RecordForContactForSync` still decides *what a served
contact contains*, unchanged and in the same query path. Scope never widens what sensitivity allows, and sensitivity
never changes scope. No new `Record` builder is added, so `backend/models/contact_record_outward_census_test.go` is
unaffected.

**Tokens and ETags.** Since no sync-token or ctag exists, a scope change is communicated *only* by a contact appearing
in or vanishing from the listing:

- **Narrowing:** contacts that leave the scope are absent from the next PROPFIND; the client's ETag comparison treats
  them as deleted on the server and removes its local copy. **That is a client-side deletion only.** The server sends
  nothing destructive, deletes no row, and writes no tombstone.
- **Widening:** contacts that enter the scope appear as new hrefs with their unchanged ETag and are downloaded.
- A scope change does **not** bump any contact's `revision`, `updated_at` or ETag. Adding or removing a circle member
  does not either (membership is a separate table); presence in the listing is the signal.
- **Soft-delete and tombstones:** a trashed contact is excluded by GORM's `deleted_at` filter before the scope
  predicate matters; the server keeps no CardDAV tombstone table (the "tombstone" in ADR 0037 is the inbound
  subscription link). A restore (ADR 0037) re-creates the contact's `circle_members` rows from the snapshot, so it
  re-enters the scope if and only if at least one of its circles is still in it. A contact restored with none of its
  circles in the scope is live but stays off the phone; the restore response already reports skipped rows and the UI
  should say it is not syncing.

**Forward constraint if sync-token/ctag is ever added.** The sync collection must (a) fold
`carddav_sync_scopes.updated_at` and the scope circles into the token so any scope edit invalidates outstanding tokens
(`valid-sync-token` precondition, forcing a full re-list), and (b) report a contact that left the scope as a `404`
member exactly as it would report a deleted one. Without (a) a narrowing would be invisible to an incremental client.

### 5. Writes and deletes

All scope checks use the same predicate and run **before** the `If-Match` ETag comparison, so a `412` can never be used
to probe an out-of-scope resource.

- **A. PUT of an existing in-scope contact:** unchanged.
- **B. PUT of an existing contact outside the scope: `403 Forbidden`.** Nothing is written. A client cannot change a
  contact's circles over CardDAV (circles are CRM-only and `PreserveLocalOnlyCRMFields` keeps them), so it cannot
  edit a contact into scope either.
- **C. PUT that creates a new contact** (UID not found among the user's live contacts) in `include` mode, where a new
  contact has no circles and would fall outside the scope, adds it to `default_circle_id` in the **same transaction**
  as the contact insert, so it stays visible on the phone. The scope is read inside that transaction: SQLite write
  transactions are serialized (`_txlock=immediate`), so the default circle cannot be deleted between the check and the
  insert. If no default circle exists (it was deleted, decision 3), the create is refused with `403` and nothing is
  written.
- **D.** In `exclude` mode a new contact is already in scope; if a default circle is configured it is also added (it is
  validated to be non-excluded), so the user can tag phone-created contacts. In `all` mode nothing is added.
- **UID reuse:** a PUT whose UID matches only a *trashed* contact creates a new live contact (current behaviour, ADR
  0037 decision 6) and then follows C or D.
- **DELETE of an in-scope contact:** a normal soft delete, but through `services.DeleteContact` (cascade registry and
  trash snapshot, ADR 0035/0037) instead of today's bare `db.Delete`. That fix is #2249 and precedes this work; the scope
  work assumes it and only adds the out-of-scope 404.
- **DELETE of an out-of-scope or unknown resource: `404`**, with no side effect. This matches
  `GetAddressObject`/`DeleteAddressObject`, which already answer `404` for a missing card (the TEST-09 interop finding
  in `backend.go`: DAVx5, Apple Contacts and vdirsyncer read `404` as "already gone"). It also matters operationally:
  after narrowing, a phone that still holds an out-of-scope contact and deletes it locally before syncing sends this
  DELETE, and a `403` would put the client in a retry loop.

**Why 403 for PUT but 404 for GET and DELETE, and the existence-oracle question.** Every lookup keeps `user_id = ?`,
so none of these responses reveal anything about another user. The only disclosure is to the account's own client: a
PUT to a UID returns `403` when that UID is an existing out-of-scope contact and `201`/`204` otherwise. That is
deliberate and accepted. The maintainer decision requires an explicit refusal for edits; a silent `404` on PUT would be
a data-loss trap because the client would retry creating a duplicate. The authenticated owner can already see every
contact in the web UI, and scope is a sync-sizing control, not an access-control boundary or a way to hide a contact
from the account holder. GET, HEAD, multiget and DELETE return `404` because, as far as that collection is concerned,
the resource does not exist; those are the verbs a client issues for a stale local copy, and conflating "out of
scope" with "gone" is the interop-correct answer. The `412` cannot leak because the scope check precedes it.

**Race after narrowing.** If a user edits a contact on the phone and the scope is narrowed before that edit syncs, the
PUT gets `403` and the next listing removes the contact locally, so the phone edit is lost (the server copy is
intact). This is accepted; the preview and the upgrade note say so.

### 6. Discovery and client implications

Nothing changes in discovery, capabilities, address-book path or advertised versions. Practical consequences:

- **The first sync after narrowing deletes those contacts on the device.** DAVx5 and Apple Contacts both compare the
  listing against local state and remove what is missing, so the contacts disappear from the phone's own Contacts app.
  This is the intended behaviour. Operator and user documentation must say so before the feature ships: a section in
  `docs/carddav.md` and an `## Upgrade notes` entry in the release PR.
- Widening re-downloads the contacts; a large widening is a large first sync.
- Existing installs and clients see no change on upgrade (no row means `all`).
- A phone-created contact in `include` mode lands in the default circle, so it shows in the web UI under that circle.

### 7. Web settings UI

A "CardDAV sync scope" card on the Settings page, shown only when CardDAV is enabled: a three-way choice (All / Only
selected circles / All except selected circles), a multi-select over the user's circles (id-backed, colours from ADR
0039), and a default-circle select constrained to the legal values for the mode. It shows the live "N of M contacts
sync" and the preview count before save, the warnings banner from decision 3, and a confirm step when saving would
remove contacts from devices. Its help text states that contacts pulled in by an inbound CardDAV subscription have no circle and so are not synced under "Only selected circles". Android and other REST clients are unaffected; they do not use CardDAV.

## Alternatives considered

- **One address book per selected circle.** Rejected by the maintainer and by decision 1: duplicate UIDs across books
  and delete-plus-create on every circle move.
- **Accept an out-of-scope create and leave it invisible.** Rejected: the phone would create a contact that
  immediately vanishes on the next sync.
- **Reject an out-of-scope create instead of filing it.** Rejected: the user loses data entered on the phone.
- **`404` for an out-of-scope PUT.** Rejected: the client creates a duplicate on retry (decision 5).
- **Fall back to `all` when the include list empties.** Rejected: it silently publishes the whole book to a device.
- **A JSON column on `users`.** Rejected: no referential integrity, and a circle delete would need a read-modify-write.
- **Filtering in Go after loading.** Rejected: contrary to the sensitivity-in-the-query rule and slower.

## Consequences

- Two additive tables with a symmetric down migration. **Down-migrating drops the scope, which reverts every user to
  `all` and widens what the next sync serves**; the release note for the version that ships it says so.
- `DeleteCircle` stays correct without new code (FK cascade); its web confirm dialog gains a pre-flight check.
- For modes other than `all` the predicate adds a correlated `EXISTS` on the indexed `circle_members.member_vcard_uid`;
  the large-dataset job should cover an `include` scope over a large book.
- Threat model: no new trust boundary. The settings endpoint is owner-scoped by `user_id`; the DAV checks reuse the
  authenticated principal.

## Test plan (the implementing issues must add these)

Backend, DB-backed (`dbtest`), through the real Gin DAV handler where the status code matters:

1. **List** (PROPFIND depth 1) and **filter-less and filtered addressbook-query REPORT** return exactly the in-scope
   set for `all`, `include` and `exclude`; a contact in two circles appears once; a contact with no circles is out for
   `include` and in for `exclude`.
2. **Multiget and GET/HEAD/resource PROPFIND** of an out-of-scope href return `404`; the same for the numeric-id
   fallback.
3. **PUT:** existing in-scope succeeds; existing out-of-scope is `403` and writes nothing (revision unchanged),
   including with a stale `If-Match` (still `403`, not `412`); new in `include` mode joins the default circle and is
   immediately listed; new in `exclude` mode with and without a default; new in `include` mode with no default circle
   is `403` with no row created; a circle-insert failure rolls the contact insert back; PUT reusing a trashed
   contact's UID.
4. **DELETE:** in-scope runs the cascade and creates a trash snapshot (restorable); out-of-scope and unknown are `404`
   with no soft-delete; another user's UID is `404`.
5. **Narrowing and widening:** the listing before and after a scope change shows contacts leaving and entering with
   unchanged ETags, and no row is deleted or has its revision bumped.
6. **Circle deleted while in the scope:** the last included circle gone leaves an empty `include` scope that lists
   nothing and is never `all`; a deleted default circle sets `default_circle_id` null and a create becomes `403`; an
   excluded circle gone leaves `exclude` with its remaining circles; the matching `warnings` appear; the contacts
   remain in the database.
7. **Trash restore:** a restored contact re-enters the listing iff one of its circles is still in the scope.
8. **Sensitivity independence:** an above-normal address and a private personal-info entry are absent from a served
   in-scope contact in every mode; an in-scope contact is never hidden because of sensitivity; PUT still preserves
   withheld sensitive fields; a scope change never makes a withheld field appear.
9. **Settings API:** ownership (another user's circle id and an unknown id give the same `400`), every validation rule
   in decision 2, `PUT` replaces the whole object, `GET` counts and warnings, `preview` persists nothing, user delete
   removes both tables' rows, migration up/down and schema parity.
10. **Chokepoint guard:** a test asserts every `contacts` query in `backend/carddav` goes through the scope helper, so
    a new DAV path cannot skip it.

Web: component tests for mode switching, default-circle constraints, the warnings banner and the circle-delete
pre-flight. A real-client DAVx5 leg that narrows the scope and asserts the contact leaves the device is desirable once
the `docs/development/reference-client-matrix.md` harness supports an incremental re-sync (today it covers provision
and pull only).

## Settled by the maintainer

1. **Account bundle.** The sync scope does not travel in the account bundle for now; after an import it defaults to
   `all`. Including it later is additive.
2. **Subscribed contacts.** Contacts created by an inbound CardDAV subscription have no circle, so they stay out of
   `include` scopes. This is behaving as designed; the web settings help and `docs/carddav.md` state it.
