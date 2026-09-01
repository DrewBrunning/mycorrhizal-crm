---
title: Failure Injection & Chaos
parent: Development
nav_order: 7
---

# Failure Injection & Chaos (TEST-06)

This page is issue #434's written-down deliverable: the split-harness rule, the
in-process injection mechanism, the external-fault CI job, and **the fault
catalog** — the table the `v0.8.0` adversarial audit (issue #500) reviews to
find the faults this list omits.

## The split-harness rule

There are two mechanisms, and a hard rule for choosing between them:

> **If the fault can be expressed as an error returned across an existing
> interface, inject it in-process. If it requires the process or the filesystem
> to actually misbehave, it belongs in the external-fault CI job.**

The failure mode of a split harness is people guessing which side a fault
belongs on. Don't. A fault that fits through an interface (a DB error
mid-transaction, a sentinel from an integration client) is injected
in-process via the `faults` package and tested in the normal Go suite. A fault
that genuinely needs the process to die or the disk to fill (SIGKILL
mid-migration, `ENOSPC` during backup) is driven by the external job
(`.github/workflows/chaos-tests.yml`), which is slow, few, and high-value.

## The in-process mechanism: `backend/internal/faults`

Faults are inert unless armed. Production code checks a fault at a real seam
with one `faults.Hook(name)` call and routes the returned error through its
existing error path — **that path is what the injection test asserts**. An
unarmed hook is a read-lock map lookup returning nil; it changes nothing.

Arming is programmatic (the test suite) or environment-driven (the external
job):

```bash
# A subprocess the test cannot reach from inside (the external job):
MYCORRHIZAL_FAULTS=fault.name,fault:err:message,fault:pause:5s
```

| Entry form | Effect |
|---|---|
| `name` | error fault, standard `injected fault: <name>` |
| `name:err:<text>` | error fault with custom text (may contain colons) |
| `name:pause:<duration>` | pause fault: block for the duration, log a greppable marker, return nil |

In-process, tests use `faults.ArmError(name, err)`, `faults.ArmPause(name, d)`,
`faults.Disarm(name)`, and `faults.Reset()` (in `t.Cleanup`). `errors.Is`
against the armed error (or `faults.ErrInjected`) is the portable assertion.

Seams are deliberately few and at real boundaries — the migration driver's
per-migration point, an import confirm's transaction, an integration client's
request path. They are not sprinkled through business logic. Where a listed
fault has no seam yet, adding one is the work of the ticket that needs it.

## The external-fault job

`.github/workflows/chaos-tests.yml` runs the faults that must actually break a
process or a filesystem. It is gated to its own `chaos` paths filter (see
`.github/filters.yaml`), so it does **not** run on every push — only when the
injection harness itself changes, on a nightly schedule, or on manual dispatch.

The external job does not observe "it errored"; it asserts **the outcome is
defined**: recovered, or failed closed with the data intact. `PRAGMA
integrity_check` passes on the database after every interruption it performs.

## Fault catalog

Each row names the fault, its seam, what it simulates, the **defined outcome**
that must hold, and where each outcome is pinned. `in-process` tests run in the
normal Go suite; `external` tests run in the chaos job. A fault with no row
here is a gap to file, never something to silently absorb.

| Fault (env name) | Seam | Simulates | Defined outcome | Pinned by |
|---|---|---|---|---|
| `database.migration.statement` | `database/sqlite_driver.go` `Run` | Process killed between a migration's commit and its clean-mark (crash leaves the version dirty, migration applied) | The run returns an error naming the failing version + file (issue #532 gate); the DB is left **dirty** at that version with the migration's schema applied; `PRAGMA integrity_check` is `ok`; the next startup run **refuses** on the dirty flag with a typed `ErrDirtyMigration` naming the recovery (MIG-04, issues #439/#546 — never a forced boot); the operator-only `MigrateForce` (the migrate CLI's prompted `force`) recovers deterministically to the latest schema, not dirty | in-process: `database/fault_injection_test.go` `TestInjectedMigrationFaultFailsClosedAndRecovers`, `...MidFlightRecordsEvent`, `...PauseBlocksThenCompletes`; `database/migrate_failclosed_test.go` `TestMigrateForce_*`; `database/interrupted_startup_test.go` `TestInterruptedStartupKillPoints/during_migration`, `TestInterruptedStartupCrashLoopConverges`; external: chaos job `migration-kill` |
| `database.migration.before_batch` | `database/migrate.go` `runPendingMigrations` (before `m.Up()`) | Process killed AFTER the mandatory pre-migration backup (issue #530) but BEFORE the first migration statement — the "before any migration begins" window (DEPLOY-03, issue #452) | The run returns an error; the schema is **completely untouched** — no version bump, no dirty flag, no partial DDL; `PRAGMA integrity_check` is `ok`; the pre-migration backup was already written and is a valid rollback point; a restart just **migrates normally**; repeated kill/restart cycles converge (the backup is reused, never rewritten) | in-process: `database/interrupted_startup_test.go` `TestInterruptedStartupKillPoints/before_migrations`, `TestMigrationBeforeBatchFaultLeavesSchemaUntouched`; external: chaos job `startup-interruption-kill-points` |
| `services.import.confirm` | `services/import_session.go` `Confirm` + `ConfirmVCF` transaction | DB error mid-import-confirm | The whole confirm **fails closed**: every contact row rolls back, the session stays unconsumed, a retry after the fault clears applies the same import cleanly — no silent partial state | in-process: `services/import_fault_injection_test.go` `TestConfirmInjectedDBErrorFailsClosed`, `TestConfirmVCFInjectedDBErrorFailsClosed`, `TestConfirmInjectedFaultDoesNotLeakToOtherSessions` |
| `services.immich.request` | `services/immich_client.go` `doRequest` | Unreachable / auth-expired / resource-deleted-remotely / arbitrary upstream failure | The armed sentinel crosses the request boundary unchanged; callers hit their documented error path (T42 classification, service degrade-to-cache), never a swallow or a panic; the seam is inert once disarmed | in-process: `services/immich_fault_injection_test.go` `TestImmichInjectedRequestFaultCrossesBoundaryUnchanged`, `...DoesNotPersistBeyondArm`, `...ReachesServiceDiagnostics` |
| Real `ENOSPC` during backup | `cmd/backup` → `database.BackupSnapshot` | Disk exhaustion while snapshotting | The CLI exits non-zero; the source database is untouched (`integrity_check` ok); no partial file appears at the output path (temp-then-link, fail-closed) | external: chaos job `disk-full-backup` |
| Real `ENOSPC` during a **large migration** | `cmd/migrate` on a 10k-contact floor database mounted on a 1 MiB-slack tmpfs | Disk exhaustion while the migration's row-touching backfills grow the WAL (issue #495's hand-verify: "reduce the available disk below what the migration needs") | The migration exits non-zero; the database is left **dirty at some migration with `integrity_check` ok** — never truncated, never a healthy-looking half-migration; the next startup run **refuses** on the dirty flag (MIG-04) | external: chaos job `large-migration-disk-full`. Note `RLIMIT_FSIZE` is NOT a valid in-process proxy: Linux `ftruncate` ignores it and SQLite pre-extends files with it — a real filesystem filling up is the honest test |

### Planned (filed) — same technique, consumers

These are the tickets that consume the harness above; the domain milestones own
the acceptance criteria, TEST-06 owns the technique:

- **MIG-04 / MIG-05 (#439/#440)** — fail-closed + rollback/recovery procedures
  for migrations, exercising `database.migration.statement` including the
  pause/SIGKILL window.
- **CON-04 (#459)** and **#526** — retry paths and ambiguous failures (the
  write succeeded but the response was lost), injected in-process.
- **#498** — constrained resources; **#530** — the mandatory pre-migration
  backup, whose failure path fails the upgrade closed
  (`ErrPreMigrationBackupFailed`); the backup seam exercises it.
- The remaining integration clients (Paperless, Seafile, WebDAV, CardDAV/CalDAV,
  webhooks, notification channels) take the same seam shape as
  `services.immich.request`; their sentinel handling is already pinned by the
  httptest-based fake-server suites in `services/`.

## Hand-verification

Per CLAUDE.md, an injection test must prove it pins its recovery path: remove
the refusal and confirm the test fails, then restore. Done for the migration
seam: deleting the dirty refusal in `database/migrate.go`
`checkMigrationPreflight` fails `TestInjectedMigrationFaultFailsClosedAndRecovers`
on its second `MigrateUp` (the dirty database recovers instead of refusing) and
`TestDirtyDatabaseRefusesToStart` (the refusal is no longer a typed
`ErrDirtyMigration`). The equivalent exercise for the import seam is removing
the `txErr` check that returns `apperrors.ErrDatabase` — the injected error then
surfaces as a success with zero rows, and the test's "must fail closed"
assertions fail.

Done for the `database.migration.before_batch` seam (DEPLOY-03): deleting the
`faults.Hook(faultMigrationBeforeBatch)` call in `runPendingMigrations` fails
`TestInterruptedStartupKillPoints/before_migrations` and
`TestMigrationBeforeBatchFaultLeavesSchemaUntouched` — the armed fault no longer
aborts the run, so the database migrates instead of staying untouched. Restore
it. The readiness backstop added alongside (schema-ahead-of-binary is
`not_ready`, not `ok`) is pinned by `controllers/health_endpoints_test.go`
`TestReadiness_SchemaAheadOfBinary`; reverting the `applied > latest` branch in
`readinessMigrations` fails it.
