# Repairing contacts deleted over CardDAV before #2249

Before issue [#2249](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2249)
was fixed, a CardDAV `DELETE` soft-deleted the contact row with a bare
`db.Delete` and skipped the normal contact cascade. The contact therefore had:

- **no trash snapshot**, so it never appeared in Trash and could not be
  restored through the API (ADR 0037); and
- **live soft-deleted dependants** (notes, reminders, gifts, life events, …)
  plus **leftover hard-join rows** (relationship edges, circle and household
  memberships, tags, field overrides, …).

The rows are purged after `DELETED_RETENTION_DAYS`, but until then the contact
is invisible in Trash while its dependants remain visible. This runbook is the
one-time, operator-run repair. It is **not** automatic: nothing runs it at
startup.

Who should run it: an operator upgrading an instance that accepted CardDAV
deletes before the release containing #2249. Run the dry run once after the
upgrade and apply it if it reports contacts.

## Before you start

1. **Take a backup** using the normal three-piece procedure in
   `docs/deployment.md`. The repair writes to the live database; a restore is
   the only rollback.
2. Run `doctor` against a **copy** first if you want a rehearsal. The command
   never needs the server to be stopped, but it does take the database's write
   lock per contact.
3. The database must already be at the current schema (start the server or run
   the migration command first). `doctor` does not migrate.

## Dry run (default, writes nothing)

From a checkout of this repository with a Go toolchain:

```sh
cd backend
SQLITE_DB_PATH=/path/to/mycorrhizal.db go run ./cmd/doctor -repair-dav-deletes
```

The output lists each affected contact with the per-table leftover counts it
would change, then says how many contacts and rows would be repaired. It writes
nothing, including no operational event. Add `-json` for the same report in
machine-readable form.

The detection is deliberately conservative: a **soft-deleted contact with no
deletion snapshot**. That also matches a contact deleted through the REST API
before the trash feature shipped; if its cascade already ran, it reports no
leftovers and is left alone.

## Apply the missing cascade

```sh
SQLITE_DB_PATH=/path/to/mycorrhizal.db go run ./cmd/doctor -repair-dav-deletes -apply
```

Each contact is repaired in its own transaction: soft steps are tombstoned with
the **contact's original `deleted_at`** (so the retention clock does not
restart), hard-join steps are deleted, and the "Me" pointer and contact feeds
are rewritten exactly as the normal delete service does. A failure rolls back
only that contact and is reported; the rest still run. The run is idempotent —
a second `-apply` reports `nothing to repair`.

If a contact was restored or snapshotted between the scan and the apply, it is
skipped rather than touched. Live contacts and normally deleted
(snapshot-backed) contacts are never candidates.

## Optionally make the contacts restorable

```sh
SQLITE_DB_PATH=/path/to/mycorrhizal.db go run ./cmd/doctor -repair-dav-deletes -apply -write-snapshot
```

`-write-snapshot` (requires `-apply`) writes a best-effort
`contact_deletion_snapshots` row, so the repaired contact appears in Trash and
can be restored. **The snapshot is lossy by construction:** hard-join rows that
were already removed before this repair ran cannot be captured. Restore
tolerates the gap — it brings back the contact, its soft-deleted dependants, and
the join rows that were present at repair time, and leaves earlier losses lost.
The CLI says so in its output; do not treat the snapshot as a full undo.

## What to check afterwards

- Re-run the dry run: it should report `nothing to repair`.
- `doctor -repair-dav-deletes` writes one operational event per affected user to
  the system-event timeline (component `contact_dav_delete_repair`), with counts
  only; it does not touch the append-only audit trail.
- If you used `-write-snapshot`, confirm the contacts now appear in Trash.

## If it refuses

The repair uses the same storage-integrity gate as `doctor -repair` (issue
[#921](https://github.com/DrewBrunning/mycorrhizal-crm/issues/921)). If the
SQLite structural checks are not clean it exits 3 without writing and tells you
to restore from a good backup first. Do that; do not try to repair a corrupt
database in place.
