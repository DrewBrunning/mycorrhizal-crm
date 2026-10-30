# ADR 0042: Standing shares — read-only live grants

- **Status:** accepted
- **Date:** 2026-10-30
- **Decides:** issue #1998 (supersedes #159). Implementation targets v1.7.0.
- **Security review:** `docs/security/standing-shares-review.md` (required by #1998's acceptance list).
- **Related:** `docs/adrs/0004-soft-vs-hard-delete-semantics.md`, `docs/adrs/0012-canonical-database-invariants.md`,
  `docs/adrs/0030-feeds-atom-emission.md` (decision 6, undo never re-arms an export),
  `docs/adrs/0035-service-boundary-and-delete-cascade-registry.md`, `docs/adrs/0037-contact-trash-and-restore.md`,
  `docs/adrs/0038-error-reason-codes.md`, `docs/adrs/0039-circle-metadata-and-aggregate-timeline.md`,
  `docs/security/asvs-l2.md` (P13), backend/AGENTS.md traps #5–#7.

## Context

Sharing today is one-time. `backend/models/contact_share.go#ContactShare` is a frozen, field-filtered JSContact
snapshot offered from one user to another (`backend/controllers/contact_share_controller.go#CreateContactShare`
builds it with `models.RecordForContactFiltered` and `models.ApplyFieldSelection`). Later edits never reach the
recipient, there is no revoke, and only contacts can be shared. Households and families need ongoing shared
context, and #159 sketched that as "true synced contacts across users". That framing carries the T13 recipient-edit
reconciliation trap and an XL estimate. #1998 asks for the smallest version that is useful, safe, and revocable,
and for the design to be fixed before any schema work.

The one-time path is complete and keeps working. This ADR adds a second, separate mechanism.

## Decision

### 1. Live read-through, read-only

The recipient reads the granted entity through a filtered read path while the grant is active. There are no copied
rows, no recipient edits and no cross-user sync. A grant is an authorization record, not a data copy: it stores
*who may read what*, never the content. Editable copies and group/household grants are explicitly later phases and
need their own ADR; this one does not pre-build for them (no `permission` column, no group id).

Why read-through: sensitivity changes, field-selection changes, revocation, expiry and entity deletion all take
effect on the next request with no propagation job, and there is nothing to reconcile.

### 2. A new `ShareGrant` resource, separate from `ContactShare`

Existing one-time shares keep working unchanged and their frozen payloads are never mutated. A one-time share is
never converted: to move from one-time to standing the owner creates a new grant (the Share dialog offers it,
decision 12). `ContactShare` gains no `standing` flag; the statuses, retention and acceptance semantics differ, and
overloading it is the trap #159 names.

### 3. Same instance, per user, no public links

A grant names exactly one recipient account on this instance. It is revocable and optionally expiring. There are no
public, anonymous or token links. (Contact feeds, ADR 0030, remain the only token-bearing export.)

### 4. Entity scopes

| `entity_type` | `entity_id` | What the recipient can read |
|---|---|---|
| `contact` | `Contact.VCardUID` | the identity (name, nickname) plus only the granted `sections` |
| `note` | `Note.UUID` | the note's text and calendar date. Not its contact link. |
| `activity` | `Activity.UUID` | title, description, location, date, end date, type. Not participants, external ref or GeoPulse data. |
| `circle` | `Circle.ID` | name, description, colour, and the display names of its live members (decision 9) |

- **Sharing a contact does not include its timeline.** Notes, activities and circles each need their own grant, and
  sharing a circle does not share the aggregate circle timeline of ADR 0039 (which would expose every member's notes
  and activities).
- `entity_id` is the entity's stable portable identity (the UUID or `vcard_uid`, not the integer PK), so a bundle
  restore or ID reuse cannot repoint a grant.
- The recipient never receives a model struct. Each type has a dedicated response view built from an explicit
  allowlist of fields, so a column added to `Note` or `Activity` later is not exposed until a person adds it to the
  view (a field-census test enforces this, see the review).
- Binary content is out of scope for v1: note/activity images (#1994/#2004), attachments and photos are not served
  through a grant. Their owner-scoped routes stay owner-only.
- The contact projection is the neutral `Card` serialized as JSContact by the same exporter the one-time share uses,
  so the already-fuzzed encoder and golden fixtures are the oracle.

### 5. Field selection is re-applied on every read

For a contact grant, `sections` (a JSON array of `models.FieldSections` tokens, non-empty, explicit; there is no
"all" default) is applied on every read through `models.RecordForContactFiltered` and `models.ApplyFieldSelection`
with `IncludeSensitive` forced to `false`. Consequences:

- A section that exists in a later release is never exposed: the grant lists the sections it names, and an unknown
  stored token is ignored (narrower), never widened.
- A new *item* in a granted section (a new email, say) is visible immediately. That is the point of a live share
  and the dialog says so.

**Re-confirmation (#159).** Sensitivity is already filtered in the query (decision 6), so a field newly marked
private/secret disappears from the recipient on the next read. To satisfy "needs explicit re-confirmation before it
is shared again", the grant also carries `reconfirm_sections` (JSON). Whenever any writer changes an item in a
sensitivity-bearing section (`addresses`, `related_to`, `personal_info`, `custom_fields`, per
`models.IsSensitiveSection`) from `normal` to `private`/`secret` on a contact with a pending or active grant, that
section token is added to the grant's `reconfirm_sections`. While a token is in the set the whole section is
withheld from the recipient, so a later reclassification back to `normal` does not silently resurface the item. The
owner clears it with `POST /share-grants/{id}/reconfirm` after seeing what the section currently contains; the
action is audited. The marker is set at the single persistence point for each sensitivity-bearing item (not in
controllers), so CardDAV pulls, imports and merges that flip sensitivity are covered. Withholding the whole section
is deliberately coarse and fail-closed; the maintainer settled this granularity (no per-item tracking).

### 6. Normal sensitivity only, filtered in the query

Every grant read path filters to `normal` sensitivity in the query, independent of the grant and of the owner's
UI. There is no `include_sensitive` for grants in this version; if it is ever added it must be an explicit, audited,
per-grant opt-in with a prominent warning, in its own ADR. A reclassification to private/secret is never an
automatic upgrade of anything.

Today `Note` and `Activity` carry no sensitivity attribute, so the guarantee for them is the allowlist view of
decision 4. If a sensitivity attribute is ever added to either, the field census fails until the grant view filters
it in the query.

### 7. Acceptance: the recipient must accept

A grant starts `pending` and exposes nothing until the recipient accepts. Rationale: the recipient picker is the
instance-wide directory (P13), so a mis-typed or mis-clicked username would otherwise hand live data to the wrong
person at once; acceptance also stops an unwanted sender from pushing items into someone's "Shared with me".

Statuses and transitions (`status` is a closed set, enforced by a CHECK constraint and a model `validate` tag):

| From | To | Trigger |
|---|---|---|
| `pending` | `active` | recipient `POST /share-grants/{id}/accept` |
| `pending` | `declined` | recipient `POST /share-grants/{id}/decline` |
| `pending`, `active` | `declined` | recipient `POST /share-grants/{id}/leave` (stop receiving), or a block |
| `pending`, `active` | `revoked` | owner `POST /share-grants/{id}/revoke` |
| `pending`, `active` | `expired` | `expires_at` passes, or a pending grant older than the pending TTL (30 days) |

`declined`, `revoked` and `expired` are terminal. There is no re-accept: to restore access the owner creates a new
grant (a new row and UUID, so the history stays unambiguous). Accepting is idempotent; accepting a terminal grant is
`409`.

What a pending recipient sees: the sender's username, the entity type, and a short label computed per request
(contact display name, circle name, activity title; a note shows only "Note"). The label is not snapshotted onto the
row. This is the least that lets a person decide, and is the same disclosure the one-time share's
`ContactDisplayName` already makes.

### 8. Blocking a sender

A new tiny table `share_blocks (blocker_user_id, blocked_user_id, created_at)` with a composite primary key, hard
delete (ADR 0004: an edge row). Declining with `{"block": true}` inserts the row, ends every pending/active grant
from that sender (`declined`), and `DELETE /share-grants/blocks/{user_id}` unblocks. While blocked, a new grant from
that sender is accepted by the API and stored with the internal status `blocked`, which the sender sees as
`pending`; the recipient sees nothing and is not notified. The sender therefore cannot use create to learn they are
blocked. A blocked grant is reaped like any pending grant at the pending TTL. Per-sender caps on outstanding pending
grants apply regardless (decision 11). The maintainer settled that a block also stops one-time `ContactShare`s from that sender; that ships as a committed v1.7.0 follow-up issue, not in this ADR's first implementation, so the one-time path is unchanged until then.

### 9. Circle grants are live membership

A circle grant exposes the circle's **current** live (non-archived, non-deleted) members' display names, resolved on
each read, in the same way ADR 0039 resolves membership. Contacts added later become visible to the recipient
immediately; contacts removed disappear immediately. A frozen member list was rejected: it needs a second table (a
snapshot, contradicting "no payload snapshot"), it drifts from the circle the owner actually sees, and it makes
"remove from circle" not mean "stop sharing".

**Risk, stated plainly:** adding a contact to a shared circle discloses that person's name to the recipient with no
further prompt, and `Contact` has no contact-level sensitivity to exclude anyone. Mitigations: the grant exposes
names only (a member's details need their own contact grant); the circle UI shows "shared with N people" wherever
members are added; and the owner's "Shared by me" row shows the live member count. The maintainer settled this: live membership, names only and capped, with this risk accepted and no frozen-list option.

### 10. Lifecycle: effective immediately, not recallable

- **Revocation and expiry take effect on the next request.** The read path evaluates
  `status = 'active' AND (expires_at IS NULL OR expires_at > now)` with the injected clock, so correctness never
  depends on the expiry job; the job only moves the row to `expired`, writes the audit event and notifies.
- **Already-seen data cannot be recalled.** The recipient may have read, screenshotted or copied it. Revocation
  stops future reads; it is not a remote wipe. The Share dialog and revoke confirmation say this, and the privacy
  doc gains the same sentence. Grant responses are `Cache-Control: no-store` and carry no ETag, so a browser or
  proxy does not keep serving a revoked read.
- **Entity deletion.** Deleting or trashing the entity ends its grants: contact delete (single, bulk, merge loser)
  and note, activity and circle delete set `revoked` with reason `entity_deleted`. For contacts this is a new
  `CascadeMutate` step in `services.ContactCascadeRegistry`; for the others a shared service helper called from the
  delete path. The read path also requires the live source row, so a missed call fails closed.
- **Trash and restore (ADR 0037).** Restore does **not** re-arm access, following ADR 0030 decision 6 and ADR 0037
  decision 3 (security-relevant state is re-armed by a person). The restore response lists the grants that were
  revoked so the UI can offer to issue them again. The step is declared `SnapshotSkip` with that reason.
- **Recreated entities.** A recreated entity is a new entity with a new identity and starts with no grants. A live
  contact created with a trashed contact's `vcard_uid` (the ADR 0037 collision case) cannot inherit a grant: the
  old grant is already `revoked`, and creating a grant needs a live owner-scoped contact.
- **Source user deleted:** their outgoing grants are hard-deleted in `deleteUserCascade`; recipients lose access at
  once. **Recipient deleted:** their incoming grants are hard-deleted and the owner receives a `revoke` audit event
  with reason `recipient_deleted` before the delete, so the owner's list does not silently lose rows. This is the
  one place both admin `DeleteUser` and self-service `DeleteOwnAccount` run (`backend/controllers/user_delete_cascade.go`).
- **Purge and retention.** Terminal rows (`declined`, `revoked`, `expired`) are hard-deleted by a scheduled job after
  `SHARE_GRANT_RETENTION_DAYS` (default 30, same semantics as `CONTACT_SHARE_RETENTION_DAYS`) so the owner can see the
  outcome. Active and pending rows are never purged by retention. A row holds no content, only identities, a
  section list and timestamps; `docs/security/data-retention-lifecycle.md` and `docs/security/pii-inventory.md` get
  entries in the implementing PR.

### 11. Audit and notifications

- **Audit.** New entity token `share_grant`; operations `create`, `revoke`, `update` exist; `accept`, `decline`,
  `expire` and `reconfirm` are added with a CHECK-rebuild migration (the same mechanism migration 000081 used for
  `restore`/`purge`). Each transition is written to the trail of each party entitled to know it: the actor's, and
  the other party's when the transition changes what they can do (accept/decline/leave/revoke/expire). The entity id
  is the grant id; snapshots carry grant fields only, never entity content or labels, so the audit trail cannot
  leak source data to a non-recipient.
- **Notifications.** The recipient is notified of a new pending grant and of a revoke; the owner is not pushed on
  accept/decline (the status is visible in "Shared by me"). In-app (a pending count and badge) is required.
  Outbound channels need a general "message" send path because the current senders are reminder-shaped
  (`backend/services/notification_service.go#NotificationSender`); that is its own implementation issue and may
  ship after the in-app surface. Payloads are minimal on purpose: sender username and entity type only, never a
  contact name, note text or label, because push gateways and SMTP relays are third parties.
- **Access history.** The owner cannot see when or whether the recipient read the entity: no per-read audit and no
  `last_read_at` in this version. A read is a `GET`, and the codebase has no state-changing `GET`s (ASVS 4.2.2);
  tracking reads would also turn a sharing feature into recipient surveillance. The maintainer settled this: no owner-visible access history, and the read path stays read-only.
- **Rate limits.** Create is subject to a cap on outstanding pending grants per sender and per sender-recipient pair
  (the per-user quota mechanism, `backend/services/user_quota.go`).

### 12. API and UI shape

All under `/api/v1`, in the `protected` group (session or `api:read`/`api:write` token; not CardDAV/CalDAV, MCP, feeds
or webhooks).

| Route | Caller | Notes |
|---|---|---|
| `POST /share-grants` | owner | body: `to_user_id`, `entity_type`, `entity_id`, `sections`, `expires_at`. Entity resolved by `user_id` (contact: `vcard_uid` and `user_id`); another user's entity is `404`. Supports `Idempotency-Key`. |
| `GET /share-grants/outgoing`, `GET /share-grants/incoming` | owner / recipient | list; incoming shows `pending` and `active` only |
| `PATCH /share-grants/{id}` | owner | only `expires_at` and narrowing `sections`; widening is `409` (create a new grant). Optimistic concurrency per ADR 0008. |
| `POST /share-grants/{id}/accept`, `/decline`, `/leave` | recipient | |
| `POST /share-grants/{id}/revoke`, `/reconfirm` | owner | |
| `GET /share-grants/{id}/content` | recipient | the one recipient read route; resolves the grant and the source per request |
| `GET /share-grants/blocks`, `DELETE /share-grants/blocks/{user_id}` | recipient | |

- Every lookup is scoped by the caller's role column (`from_user_id` for owner routes, `to_user_id` for recipient
  routes) in the same `WHERE` as the id. Any grant that is not the caller's, in the wrong role, in the wrong state,
  expired, blocked or whose entity is gone returns the **same** `404 share_grant_not_found` with the same body and
  no side effect. This extends ADR 0037's "404, never 403".
- Reason codes use a new `share_grant` area added to `reasonAreas` in `backend/errors/reasons.go` in the same PR as
  its first code (ADR 0038).
- Web: the Share dialog gains a one-time vs standing choice; a "Shared with me" page (accept, decline, leave, block,
  read); a "Shared by me" page (status, expiry, revoke, reconfirm, live circle member count). Android follows as a
  separate PR per the platform split. The existing one-time acceptance flow is not overloaded.

### 13. Data model, deletion, migrations

`share_grants`, one new migration pair (`NNNNNN_share_grants.up.sql` / `.down.sql`, hand-written, next free number at
implementation time, never `AutoMigrate`):

| Column | Notes |
|---|---|
| `id` | UUID PK, generated in `BeforeCreate` |
| `from_user_id`, `to_user_id` | `NOT NULL`, FK to `users(id)` `ON DELETE CASCADE`, `CHECK (from_user_id <> to_user_id)` |
| `entity_type` | `CHECK IN ('contact','note','activity','circle')` |
| `entity_id` | text, stable identity per decision 4 |
| `sections` | JSON text, `NOT NULL DEFAULT '[]'` (empty for non-contact) |
| `reconfirm_sections` | JSON text, `NOT NULL DEFAULT '[]'` |
| `status` | `CHECK IN ('pending','active','declined','revoked','expired','blocked')` |
| `responded_at`, `expires_at`, `revoked_at` | nullable instants (ADR 0015) |
| `revoked_reason` | `CHECK` closed set, nullable |
| `created_at`, `updated_at`, `revision` | per ADR 0006/0018 |

There is no payload column and no snapshot. Indexes: `(to_user_id, status)`, `(from_user_id, status)`, and
`(entity_type, entity_id, status)` for the cascade step. A **partial unique index** on
`(from_user_id, to_user_id, entity_type, entity_id) WHERE status IN ('pending','active','blocked')` prevents duplicate
live grants while letting history rows coexist (ADR 0004's partial-index form).

**Soft vs hard (ADR 0004).** No `deleted_at`. A grant is authorization state with a lifecycle, not authored content
and not a bare edge: the `status` column is its tombstone, terminal rows are kept for the retention window and then
hard-deleted, and `DeleteUser` removes them unconditionally. `share_blocks` is an edge row: hard delete.

**Cascade.** Add `share_grants` (both columns) and `share_blocks` to `deleteUserCascade` and its structural coverage
test; add the contact `CascadeMutate` step (decision 10); call the shared revoke helper from the note, activity and
circle delete paths. `docscheck` requires the tables in the PII inventory.

**No mass assignment.** `POST` binds a DTO with only `to_user_id`, `entity_type`, `entity_id`, `sections`,
`expires_at`; `PATCH` binds only `expires_at` and `sections`. Status, `from_user_id`, timestamps, revision,
`reconfirm_sections` and `revoked_*` are set only by the service. Updates use explicit column allowlists.

**Tests** use `dbtest.New` (the real migrated schema), register the model in `schema_parity_test.go`, and include a
symmetric down migration test and a populated-database upgrade check.

### 14. Interaction with the rest of the app

- **#1997 per-contact field visibility is cosmetic.** It hides fields in the owner's own UI and is never an access
  boundary: a grant read neither consults nor honours it, and it is not consent. The Share dialog previews the
  actual grant projection so a UI-hidden field that is in a granted section is not a surprise.
- **ADR 0039 circles.** Circle grants use the metadata and live membership of decision 9; the aggregate timeline is
  not shared.
- **P13 directory.** The picker keeps using `GET /users/directory`; no new exposure is added. Create returns `404`
  for an unknown `to_user_id`, which the directory already makes discoverable. The pending-grant spam and
  wrong-recipient risks P13 raises are addressed by acceptance (7), blocking (8) and the caps (11). A restricted
  directory stays follow-up #2218.
- **Outward surfaces.** Granted data is excluded from the recipient's contact list, search and FTS, timeline,
  reminders, graph, dedupe, exports, account bundle, change feed, CardDAV/CalDAV, MCP, webhooks and feeds. The grant
  read route is added to the outward census (`backend/models/contact_record_outward_census_test.go`). The account
  bundle excludes grants because user ids do not transfer between instances.

## Alternatives considered

- **Copy-on-grant with a sync pair.** Rejected: it brings the T13 reconciliation problem, duplicates sensitive data,
  and makes revocation a recall.
- **A `standing` flag on `ContactShare`.** Rejected (decision 2).
- **Active immediately, no acceptance.** Rejected: wrong-recipient exposure via the directory (decision 7).
- **Convert a one-time share in place.** Rejected: the frozen payload is a different consent object.
- **Per-grant `include_sensitive`.** Deferred, not rejected (decision 6).
- **Soft-delete grants.** Rejected: revoked rows would still occupy the live-grant unique key without a partial index,
  and `status` already carries the lifecycle.

## Consequences

- Two new tables, one new audit entity and four audit operations, one reason-code area, one retention variable, one
  scheduled job (expiry + pending TTL + terminal purge).
- Every entity delete path gains a revoke call; a completeness test over the delete paths prevents a future entity
  type from being grantable without one.
- Reads add one indexed grant lookup and one scoped source lookup, inside a single read transaction.
- Recipients can keep what they saw. This is stated in the UI and privacy doc, not engineered around.
- Implementation is split backend first, then web, then Android, each tied to #1998. A candidate issue list is in the
  PR that lands this ADR.

## Settled by the maintainer

1. **Circle membership is live**, names only and capped, with the disclosure risk of decision 9 accepted. There is no
   frozen-list option.
2. **No owner access history.** The owner sees no per-read audit and no `last_read_at`; the read path stays read-only.
3. **Re-confirmation withholds the whole section** until the owner re-confirms (decision 5). No per-item tracking.
4. **A block also stops one-time `ContactShare`s from that sender.** This is a committed v1.7.0 follow-up issue, not
   part of the first implementation of this ADR.

## Open questions for the maintainer

1. **Pending TTL and the pre-acceptance label.** Not yet decided. Recommendation: keep the 30-day pending TTL (long
   enough to answer, short enough that stale offers do not linger) and keep showing the label (contact display name,
   circle name, activity title; a note shows only "Note"), because a person cannot make an informed accept without it.
   The cost is that a mis-addressed recipient sees a contact's display name. The stricter option is "A contact" until
   accepted.
