# Capacity under constrained resources (issue #498)

This is the written-down answer to the question milestone `v0.6.9` poses
directly: **when a resource runs out, what does each operation do?** For every
`constraint × operation` cell, one of four outcomes:

| Code | Outcome | Acceptable? |
|---|---|---|
| **C** | **Completes** — the operation succeeds despite the constraint (slower is fine). | yes |
| **D** | **Degrades** — a preflight check or an explicit cap refuses up front with a clear, actionable error; nothing is half-done. | yes (best) |
| **F** | **Fails closed** — the operation fails mid-flight, rolls back, surfaces an error; the data is intact and there is no silent partial result. | yes |
| **X** | **Corrupts / silently loses data.** | **never** |

`X` is the only unacceptable outcome, and it is the default unless someone
checks — which is why this table exists and is backed by tests. The rule from
[fault-injection.md](fault-injection.md) applies: a fault that fits through an
error interface is pinned in-process by the normal Go suite; a fault that needs
the filesystem or the kernel to actually misbehave is pinned by the
external-fault job (`.github/workflows/chaos-tests.yml`).

This complements [scale-profiles.md](scale-profiles.md) (the representative
datasets and their *normal-condition* resource cost) and
[scale-testing.md](scale-testing.md) (the migration path's recorded resource
requirements). Network failure is a separate axis owned by INT-02 (issue #465,
`services/*_failure_behavior_test.go`) and is not duplicated here.

## The matrix

Columns: **Disk** = the filesystem holding the database fills up (`ENOSPC`);
**Memory** = a tight `MemoryMax` cgroup / an OOM-prone box; **CPU** = a
throttled CPU (`CPUQuota`); **Slow storage** = high I/O latency / lock
contention; **Concurrency** = many simultaneous writers; **Many users** = tens
of accounts whose per-user background jobs multiply against one SQLite writer.

| Operation | Disk | Memory | CPU | Slow storage | Concurrency | Many users |
|---|:--:|:--:|:--:|:--:|:--:|:--:|
| Ordinary write (`POST/PUT/DELETE /contacts`) | F | C | C | C | C | C |
| Wizard import confirm (CSV / VCF / JSContact) | **D** → F | C¹ | C | C | n/a | n/a |
| Source import (Meerkat / Monica) | **D** → F | C¹ | C | C | n/a | n/a |
| Full-database export (CSV / VCF / JSContact) | C² | **D** | C | C | n/a | n/a |
| Online backup (`VACUUM INTO`) | **D** → F | C | C | C | C | n/a |
| Startup migration | **D** → F | C / F³ | C | C | n/a | n/a |
| FTS index rebuild | F | C | C | C | C⁴ | n/a |
| Large read query (graph hub, duplicate scan) | C² | F⁵ | C | C | C | C |
| Concurrent request load | F | C | C | C | C | C |
| Per-user scheduled jobs (cadence, reach-out, retries) | F | C | C | C | C | C |

1. Import reads the whole upload into memory but is bounded up front —
   `MaxCSVSize` 20 MB / `MaxCSVRows` 20 000, `MaxVCFSize` 50 MB /
   `MaxVCFContacts` 20 000, `MaxMeerkatDBSize` 100 MB — so a single import
   cannot grow memory without limit (issue #415).
2. `ENOSPC` does not apply: exports stream to the HTTP response and read-only
   queries do not write. The memory cost is the real constraint — see the
   Memory column.
3. A migration under a memory cap either **completes** (the migration engine
   reads through the DB, it does not hold it — peak RSS ~35 MB even at 100 k
   contacts, see [scale-testing.md](scale-testing.md)) or, if OOM-killed, is
   left **dirty and refuses on the next start** (MIG-04, issue #439) — never a
   healthy-looking half-migration.
4. `RebuildSearchIndexExclusive` serialises rebuilds in-process; SQLite
   `_txlock=immediate` serialises the rebuild transaction against every other
   writer, so a concurrent rebuild cannot corrupt the index (issue #461).
5. Graph-hub traversal and `duplicates.find_pairs` buffer an unbounded result
   set (`services/graph_traversal.go`, `services/duplicate_service.go`). They
   are **read-only**, so an OOM kill there cannot corrupt the database — the
   process dies, the next request works, `PRAGMA integrity_check` is `ok`.
   Bounding those result sets is owned by #470/#471 (budgets); this ticket
   only asserts the OOM is non-corrupting.

"**D → F**" means: the preflight check *degrades* (a clear refusal before
anything is touched), and if the preflight is skipped or a racing process eats
the space in the gap, the operation still *fails closed* — the fail-closed path
is the backstop, not the only guard.

## Where each cell is pinned

### Disk exhaustion

| Cell | Pinned by |
|---|---|
| Backup — preflight refusal + fail-closed backstop | in-process: `database/backup_preflight_test.go` (`TestBackupSnapshot_RefusesWhenDiskTooFull`, `..._DoesNotBlockWhenStatfsUnreadable`); external: chaos job `disk-full-backup` |
| Pre-migration backup on a full disk → upgrade refuses | in-process: `database/backup_preflight_test.go` `TestInitDB_PreMigrationBackupFailsClosedOnFullDisk`; external: chaos job `large-migration-disk-full` (scenario A) |
| Migration itself hits `ENOSPC` → dirty, restart refuses (MIG-04) | external: chaos job `large-migration-disk-full` (scenario B), `mem-limited-migration` |
| Wizard / source import — preflight 507, session unconsumed, 0 rows | in-process: `services/import_preflight_test.go` |
| Ordinary bulk write hits `ENOSPC` → fails, never corrupts | external: chaos job `disk-full-write` |
| FTS rebuild hits `ENOSPC` → rolls back, prior index intact | in-process: `services/capacity_fault_injection_test.go` `TestSearchIndexRebuild_InjectedFaultLeavesPriorIndexIntact`; external: chaos job `disk-full-fts-rebuild` |

The free-space primitive is `backend/internal/diskspace` (`diskspace.Require`,
`ErrInsufficientSpace`). A statfs that cannot be read is **not** treated as a
failure — the preflight is a best-effort guard in front of an operation that
keeps its own fail-closed path.

### Limited memory

| Cell | Pinned by |
|---|---|
| Full-database export beyond the safety ceiling → 507 | in-process: `controllers/export_limit_test.go`. `MaxExportContacts` = 250 000, mirrors `MaxAuditExportRows` |
| Migration under a 128 MiB cap → completes or refuses, never corrupts | external: chaos job `mem-limited-migration` |
| Large read query OOM → read-only, no corruption | analytic (note 5) + `mem-limited-migration`'s integrity assertion covers the shared-writer case |

### Limited CPU

| Cell | Pinned by |
|---|---|
| Seed + migrate + FTS rebuild under a 20% `CPUQuota` stay correct | external: chaos job `cpu-limited-correctness` (exact row counts, `integrity_check=ok` + `foreign_key_check`) |

### Slow storage / lock contention

No dedicated CI job — a real latency-injecting block device is impractical on
GitHub runners. SQLite surfaces slow storage as longer-held write locks, i.e.
more `SQLITE_BUSY` pressure, which **is** reproduced:

- `backend/database/concurrent_write_test.go` — `_txlock=immediate` +
  `busy_timeout` make concurrent writers queue, never error (CLAUDE.md
  backend trap #9).
- `backend/internal/perfbench` `TestConcurrentWriteShape` — the in-process
  analogue.
- `backend/cmd/loadsmoke` against the deployed artifact (see below).

### Concurrency & many users

| Cell | Pinned by |
|---|---|
| Many concurrent writers → no `SQLITE_BUSY`, no spurious 429 | `backend/cmd/loadsmoke` (issue #262), run per the `e2e-tests.yml` "concurrency/load smoke test" step |
| Many *users* → per-user job fan-out is contention-safe, integrity holds | in-process: `services/scheduler_contention_test.go`; external: `e2e-tests.yml` "many-users concurrency smoke test" (`LOADSMOKE_USERS=10`) + "Assert the database is not corrupt after the load" (`PRAGMA integrity_check`) |

`loadsmoke` gained `LOADSMOKE_USERS` (spread workers round-robin across N
authenticated sessions) and `LOADSMOKE_DB_PATH` (post-run
`PRAGMA integrity_check` where the tool can see the database file).

## Minimum viable resources

Operator-facing sizing. Re-measure before quoting a number in a release note;
the figures below are from [scale-testing.md](scale-testing.md)'s recorded
measurements and the chaos-job comments.

### Free disk space

| Operation | Needs free, on the filesystem holding the DB |
|---|---|
| Online backup (`VACUUM INTO` / `make backup`) | **≥ 1× the `.db` size** (a full compacted second copy) + ~10% or 16 MiB headroom. The preflight (`database.backupSpaceEstimate`) asks for exactly this; below it, `make backup` refuses with `ErrInsufficientSpace`. |
| Pre-migration backup (issue #530, taken automatically on every upgrade) | Same as backup, at `MYCORRHIZAL_PRE_MIGRATION_BACKUP_DIR` (default: a `pre-migration/` sibling of the DB). If it cannot be written the **upgrade refuses** — fail-closed is correct. |
| Startup migration (transient WAL / table-rebuild growth) | **Peak additional disk** on top of the `.db`: ~16 MB at 10 k contacts, ~33 MB at 20 k, **~184 MB at 100 k** (`v0.6.0 → current`; every shorter path is cheaper). |
| Bulk import | Roughly `staged rows × 8 KiB + 32 MiB` — the `preflightImportDiskSpace` estimate. Below it the confirm returns `507 Insufficient Storage` with the session left intact for a retry. |

**Rule of thumb:** a deployment should keep free space ≥
`largest of {DB size (for a backup), peak migration additional disk}` — for a
100 k-contact instance that is **~450 MB** (a full `.db` copy), comfortably
above the ~184 MB migration peak.

### Memory

Peak RSS is dominated by SQLite's page cache and each operation's working set,
not the dataset:

| Operation | Peak RSS | Floor |
|---|--:|---|
| Startup migration | ~30–35 MB across 2 k → 100 k contacts | A **128 MiB** container migrates 100 k contacts with room to spare (`mem-limited-migration`). |
| Core read/write operations | bounded query shapes, `LIMIT 50` result sets (`internal/perfbench/baseline.json`) | 128–256 MiB is comfortable at the `large` profile. |
| Full-database export | proportional to contact count (whole payload buffered) | Capped at `MaxExportContacts` = 250 000; a real personal CRM (`typical` = 900, `large` = 15 000/user) is nowhere near it. |
| Contact / VCF / CSV import | bounded by the upload-size caps in note 1 | The 50 MB VCF cap is the worst case. |

### CPU

No minimum — every operation stays **correct** under a 20% `CPUQuota`
(`cpu-limited-correctness`), just slower. Migration/seed/FTS-rebuild wall-clock
scales roughly inversely with the quota.

## Hand-verification (per CLAUDE.md)

Each guard must prove it moves the outcome. Remove it, confirm the cell drops
a category, restore.

- **Backup preflight.** Delete the `diskspace.Require` block in
  `database/backup.go` `BackupSnapshot`. `database/backup_preflight_test.go`
  `TestBackupSnapshot_RefusesWhenDiskTooFull` fails — the error is no longer a
  typed `ErrInsufficientSpace` and no longer contains `"backup preflight"`; the
  Disk × Backup cell drops from **D** to **F** (the temp-then-link path still
  fails closed, but there is no clean up-front refusal). Restore.
- **Import preflight.** Delete the `preflightImportDiskSpace` call in
  `services/import_session.go` `Confirm`. `services/import_preflight_test.go`
  `TestConfirm_RefusesWhenDiskTooFull` fails — the confirm now enters the
  transaction and fails deep inside it instead of returning a 507 up front;
  Disk × Import drops **D** → **F**. Restore.
- **Export cap.** Delete the `guardExportContactCount` call in `ExportData`.
  `controllers/export_limit_test.go` `TestExport_RefusesWhenContactCountExceedsLimit/csv`
  fails — the handler now loads and buffers the whole dataset; Memory × Export
  drops **D** → **F** (OOM). Restore.
- **FTS rebuild seam.** Point `faults.Hook(faultSearchRebuild)` in
  `services/search_service.go` at a name the test never arms (or delete the
  line). `services/capacity_fault_injection_test.go`
  `TestSearchIndexRebuild_InjectedFaultLeavesPriorIndexIntact` fails — the
  armed fault no longer aborts the rebuild, so it completes instead of rolling
  back. Restore.
- **Source-import seam.** Same for `faults.Hook(faultImportSource)` in
  `services/import_source.go` `executeSourceImport` —
  `TestSourceImport_InjectedFaultFailsClosed` fails the same way. Restore.
- **External.** Per the chaos-job convention: raising `disk-full-fts-rebuild`'s
  tmpfs slack until the rebuild *succeeds* makes the "prior index survives"
  assertion moot; `disk-full-write` with a large enough tmpfs stops exercising
  `ENOSPC` at all. These are guarded by the `::error::` checks in the jobs
  ("succeeded on a … disk").
