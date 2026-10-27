# ADR 0037: Contact trash — restore from a deletion snapshot

- **Status:** accepted
- **Date:** 2026-10-27
- **Implements:** issue #2002 (the restore-semantics decision its acceptance list requires before schema work).
- **Related:** `docs/adrs/0004-soft-vs-hard-delete-semantics.md`, `docs/adrs/0035-service-boundary-and-delete-cascade-registry.md`
  (the delete-only `ContactCascadeRegistry` this builds on), `docs/adrs/0006-revision-token-schema.md`,
  `docs/adrs/0012-canonical-database-invariants.md`, backend/AGENTS.md traps #5–#7.

## Context

`DeleteContact` soft-deletes the contact and runs `ContactCascadeRegistry`; `PurgeSoftDeletedRows` hard-deletes
past `DELETED_RETENTION_DAYS`. The user has no way to list or restore a soft-deleted contact, although the row
survives. Restoring is harder than flipping `deleted_at` because the cascade mixes two kinds of step:

- **soft** steps (notes, reminders, reminder completions, life events, preferences, conversation agenda, gifts,
  cadence and data-decay policies, occasion obligations, attachments) keep their rows with `deleted_at` set;
- **hard** steps (`activity_contacts`, `relationship_edges`, `household_members`, `circle_members`,
  `contact_tags`, `field_values`, `contact_sync_links`, `external_*`, `reach_out_suggestions`,
  `life_event_suggestion_resolutions`, `occasion_event_attendees`, `dismissed_duplicate_pairs`,
  `import_source_links`, `notification_deliveries`, `contact_sync_conflicts`) physically remove rows that cannot
  be revived, plus two **mutate** steps (the `users.self_contact_vcard_uid` pointer cleared; contact feeds revoked).

Restoring only the contact would silently drop circles, tags, relationships, field values and activity links.

Two facts in the issue text are corrected here, because the design depends on them:

1. `idx_contacts_vcard_uid_user` is **partial** (`WHERE vcard_uid IS NOT NULL AND deleted_at IS NULL`); it does *not*
   cover soft-deleted rows. A live contact can therefore be created with a trashed contact's UID (CardDAV/import
   re-pull, vCard import of the same card). Restore must detect that collision itself (decision 6).
2. The cascade is also the reason soft-deleted children are not distinguishable by time alone: a note the user
   deleted a week *before* the contact is indistinguishable from one the cascade deleted, other than by timestamp.

## Decision

### 1. Restore is driven by a per-deletion snapshot (option 1), not per-step `Restore` code

A new table `contact_deletion_snapshots` holds one row per contact deletion, written **in the same transaction**
as the cascade (`DeleteContactInTx`), so a snapshot exists if and only if the delete committed.

| Column | Meaning |
|---|---|
| `id` | UUID PK, generated in `BeforeCreate` |
| `user_id` | owner; every query is scoped by it (trap #5) |
| `contact_id`, `contact_vcard_uid` | the trashed contact (the snapshot outlives nothing but the contact row's purge) |
| `batch_id` | UUID stamped on every cascade-soft-deleted child of this deletion (decision 2) |
| `deleted_at` | the deletion instant from the injected clock, equal to `contacts.deleted_at` |
| `payload` | JSON: the rows the *hard* and *mutate* steps removed, keyed by table (decision 3) |
| `payload_version` | integer; bumped when the payload shape changes so restore can reject or migrate old snapshots |

The snapshot is **user data** and is covered like the contact: it is deleted with the contact on purge, and by the
user-delete cascade (`deleteUserCascade`). It is encrypted at rest with the same serializer as other
sensitive payloads, because it carries copies of rows that may include `private`/`secret` field values.

### 2. A deletion batch marker, not a time window

The cascade stamps `deletion_batch_id` on each soft-deleted child it removes (a nullable column on each soft
step's table, added by the same migration). Restore revives only rows whose `deletion_batch_id` equals the
snapshot's `batch_id`. A row the user deleted before the contact has `deletion_batch_id IS NULL` and stays
deleted; a row soft-deleted by a different earlier contact delete (a merge loser) carries a different batch. A
time window (the fork's approach) is rejected: it resurrects content deleted in the same second and drops
content under clock skew.

### 3. What the snapshot carries, and what is deliberately not restored

Restored from the payload (re-created as new rows, same natural keys):
`activity_contacts`, `relationship_edges` (only edges whose other endpoint still exists and is live; otherwise
skipped and reported), `household_members`, `circle_members`, `contact_tags`, `field_values`,
`external_identities`, `external_activities`, `occasion_event_attendees`, and the `users.self_contact_vcard_uid`
pointer **only if it is still null**.

Not restored, by design — each is system-generated or security-relevant, and regenerates or is re-armed by a
person:

- `notification_deliveries`, `reach_out_suggestions`, `life_event_suggestion_resolutions`, `contact_sync_conflicts`,
  `dismissed_duplicate_pairs`: derived or review-queue state; the schedulers recompute them.
- contact **feeds**: revoked at delete and stay revoked (ADR 0030 decision 6: undo must not silently re-arm an
  export). The restore response lists them so the UI can offer to re-issue.
- `import_source_links`: restored only if the source key is still unlinked, otherwise skipped (the re-import
  already claimed it).
- `contact_sync_links`: see decision 7.

Restore reports what it skipped (`skipped: [{table, reason, count}]`) so loss is visible, never silent.

### 4. Restore semantics

Restore runs in one transaction: re-check ownership and that the contact is trashed, check the UID collision
(decision 6), clear `contacts.deleted_at`, un-delete the batch-stamped children, re-insert the payload rows,
then recompute derived state. It bumps the contact's revision token (ADR 0006/0018) and `updated_at` so ETags
and the `?since=` change feed see the restore; revived notes get the same `updated_at` bump the delete applied.
The contact's flat denormalized columns and `contacts_fts` are rebuilt through the existing model hooks / derived
rebuild path (trap #2), not by raw SQL. An entity restored is audited (`contact.restored`, decision 8).

A snapshot whose `payload_version` the running binary does not understand makes restore return `409
snapshot_unsupported`; the contact stays trashed (still purgeable) rather than being half-restored.

### 5. Purge

Purge (single, empty-trash, and the scheduled `PurgeSoftDeletedRows`) hard-deletes the contact, its
batch-stamped soft-deleted children, and its snapshot in one transaction. On-disk photo and attachment files are
removed only **after** commit, and a file-removal failure is logged and left for the orphan sweep, never
rolled back into the database. Retention reports `DELETED_RETENTION_DAYS`; `0` means keep forever, matching the
scheduled purge (a user "delete forever" still works at `0`).

### 6. UID conflict is a 409

If a live contact of the same user already holds the trashed contact's `vcard_uid`, restore returns
`409 Conflict` (`contact_uid_in_use`) naming the live contact's id, and changes nothing. There is no auto-merge
and no UID rewrite: the user resolves it by merging or deleting the live contact. A purged contact has no
snapshot and nothing to restore, so a purged UID is simply free.

### 7. CardDAV / sync interaction

- The delete currently hard-deletes `contact_sync_links`. With snapshots, the link row is captured in the
  payload and **restored** with it (same `subscription_id`, `href`, `etag`, `content_hash`, `synced_values`), so a
  restored contact re-enters sync as the same linked object and the next pull updates it instead of duplicating.
- Between delete and restore, a pull that finds the remote object with no link would re-import a new contact.
  To prevent this, a deleted contact's link is **kept and marked** (`contact_sync_links.tombstoned_at`) instead of
  being hard-deleted: sync treats a tombstoned link as "known, ignore on pull", and the link is removed only by
  purge. This changes `contact_sync_links` from the ADR 0035 hard step to a *mutate* step; the registry,
  completeness test and fault test are updated accordingly.
- The CardDAV **server** already reports soft-deleted contacts as removed in its sync report; a restore bumps the
  contact's `updated_at`/ETag so clients see it as added again.
- Purge leaves the tombstoned link until the link's subscription is deleted, so a purge-then-pull is ignored; the
  link is never re-pointed at a restored UID.

### 8. API, webhook, audit

Routes under `/api/v1/trash/contacts` (list, restore, purge, empty), owner-scoped by `user_id`; another user's id
is `404`, never `403`, with no side effect. New webhook event `contact.restored` is added to
`models.WebhookInput`'s `oneof` allowlist and the event registry. Restore and purge each write an audit event
(`restore`, `purge`); the purge audit row survives the contact (it is the append-only trail and carries no
copy of the contact's fields beyond the existing entity identifiers). Sensitivity filtering follows the owner: the
trash list is an in-app owner view, like the contact map, and is listed as an owner surface in
`contact_record_outward_census_test.go`.

### 9. Registry coupling

`CascadeStep` gains no `Restore` method. Instead the snapshot writer is generic: for every `hard` step with a
model it serialises the rows the step is about to delete (`Match`) into the payload before the delete runs; for
a raw join table without a model (`activity_contacts`) it uses the table's column list. Consequence: **a new
contact-dependent entity is restorable by construction** once it is a registry step; a step that must *not* be
restored (derived state) is declared `SnapshotSkip: true` with a reason, and a completeness test requires every
hard/mutate step to be either snapshotted or explicitly skipped — mirroring the existing "registered or reasoned
exemption" tests. This is the lever #2063's follow-ups (#1994/#2004, #1997, #1993, #1998, #1999) use: they add a
step and pick snapshot or skip.

## Alternatives considered

- **Per-step `Restore` companion (option 2).** Rejected: it multiplies code per entity, and a hard step that
  needs reversing needs the deleted row's contents anyway — i.e. a snapshot. It remains possible later for a
  step whose revival logic is not "re-insert the row".
- **Narrow promise (option 3).** Rejected: silently shipping a "restore" that drops circles, tags and
  relationships is the same data loss the feature exists to prevent.
- **Time-window child selection.** Rejected for the reasons in decision 2.
- **Make every join row soft-deleted.** Rejected: natural-key unique indexes make a soft-deleted row occupy the
  key (trap #7); changing that is a much larger migration than a snapshot table.

## Consequences

- One new table, a nullable `deletion_batch_id` column on each soft cascade table, and one nullable
  `tombstoned_at` column on `contact_sync_links`; all additive, down migration drops them losslessly, no backfill
  needed (pre-existing trashed contacts have no snapshot).
- **Contacts deleted before the migration are listed in the trash but only partially restorable** (the contact and
  its un-batched soft children by the `deleted_at` equality used as a one-time fallback, with the join rows
  reported as lost). The list marks them `restore_scope: "partial"` so the UI warns before restoring.
- Delete cost rises by one read per hard step (already paid for audited soft steps) and one insert.
- `docs/security/data-retention-lifecycle.md` and `docs/security/pii-inventory.md` gain the snapshot table in the
  implementing PR; the user-delete cascade and `purge_completeness_test.go` must cover it.
- Implementation is split by platform: backend, web, then Android, each its own PR tied to #2002.

## Amendment (2026-10-27, backend implementation)

Three details changed while implementing, each simpler than the original text and with the same guarantees:

- **Batch marker lives in the snapshot, not on the child tables (decision 2).** `payload.soft` maps each
  soft-cascade table to the ids of the rows *this* deletion soft-deleted, captured before the cascade runs.
  Restore un-deletes exactly those ids (owner-scoped). No `deletion_batch_id` column is added to eleven
  tables, so there is no schema-parity or DTO surface to keep in step.
- **The sync link is retained, not tombstoned with a new column (decision 7).** The cascade no longer deletes
  `contact_sync_links` (a `CascadeMutate` no-op step). The pull skips a link whose contact is trashed or
  purged and keeps the link; the scheduled purge no longer removes it. That is the tombstone, and a restore
  needs no relinking. A merge loser keeps its pre-existing "discard the link" behaviour (explicit delete in
  the merge path): it is not restorable.
- **Pre-existing deletions are not listed, and the snapshot column is `trashed_at`.** A soft-deleted contact
  with no snapshot — deleted before the migration, or a merge loser — cannot be told apart, and restoring a
  merge loser would resurrect a duplicate, so the trash lists snapshot-backed deletions only (the earlier
  "partial restore" row is dropped). The scheduled purge still reaps them on schedule. The column is named
  `trashed_at` because the purge-completeness guard treats any `deleted_at` column as a soft-delete column.

Photo and attachment files are no longer removed when a contact is deleted; they are removed by every purge
path after commit (decision 5), which required changing `DeleteContact`, the bulk delete action and the
scheduled purge. Audit gained `restore` and `purge` operations (migration 000079 rebuilds the CHECK, as 000070
did).
