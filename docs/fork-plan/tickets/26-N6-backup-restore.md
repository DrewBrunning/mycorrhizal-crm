# N6 — Full backup restore

| | |
|---|---|
| **Rating** | 3 |
| **Size** | M |
| **Depends on** | — |
| **Alpha** | after |
| **Source** | New (gap found in the 2026-07-30 product review) |

## The gap

**Export is complete. Import is contacts-only.** So an exported instance cannot be restored.

- `ExportData` (`controllers/export_controller.go`) writes a combined CSV with `=== CONTACTS ===`,
  `=== RELATIONSHIPS ===`, `=== ACTIVITIES ===`, `=== NOTES ===`, `=== REMINDERS ===` sections, plus
  custom-field columns.
- `ExportContactsAsVCF` / `ExportContactsAsJSContact` cover contacts richly.
- **Import** (`import_service.go`, `import_controller.go`) handles CSV/VCF/JSContact **contacts only** —
  there is no path back in for notes, activities, reminders, or relationships.

## Scope — locked in as file-level backup/restore (option a)

**Decision locked 2026-08-04:** This ticket is file-level backup/restore only. App-level import of
remaining entity types (notes, activities, reminders, relationships) is explicitly out of scope.
If partial restore or instance migration is ever wanted, it will be its own ticket with its own
design pass — no existing user has asked for it, and the effort (genuinely L, not M) doesn't match
this ticket's R3 rating.

## What to build

Document a tested backup/restore procedure in `docs/deployment.md`, plus a `make backup` Makefile
target if useful. This is a docs + Makefile task — **no backend code, no frontend code, no
migrations, no i18n.**

The procedure must cover:

1. **Online backup** using `VACUUM INTO` — produces a consistent SQLite snapshot without stopping the
   server. WAL mode is on (`database/open.go` sets `_journal_mode=wal`), so naively copying the
   `.db` file while the server is running produces a corrupt snapshot. `VACUUM INTO` is the correct
   approach for a live instance.
2. **Offline backup** as a fallback — stop the server, copy the `.db` file and the profile-photo
   directory (`PROFILE_PHOTO_DIR`), restart. Simpler but requires downtime.
3. **Restore procedure** — stop the server, replace the `.db` file and photo directory from backup,
   restart. Include a verification step (list contacts, check a known note/reminder survived).
4. **Photo directory** — lives outside the SQLite file (`config.Config.ProfilePhotoDir`, configured
   via `PROFILE_PHOTO_DIR` env var). Must be backed up separately. Document this explicitly.

## Traps

- **WAL checkpoint before `VACUUM INTO`** — if the WAL has uncheckpointed pages, `VACUUM INTO` may
  not include the most recent writes. Run `PRAGMA wal_checkpoint(TRUNCATE)` first.
- **Attachments (N7)** — if attachments land before this ticket does, they live in their own
  directory outside SQLite and the procedure must cover that directory too. Add a note in the
  attachments ticket's "Done when" to update this procedure.
- **`make backup` target** — if built, it should shell out to `sqlite3` for `VACUUM INTO` rather
  than linking against the Go SQLite driver, because `VACUUM INTO` is a SQL statement the Go driver
  handles normally. The target reads `SQLITE_DB_PATH` from the environment (same as the Makefile
  already does for `migrate-up`/`migrate-down`).

## Done when

- `docs/deployment.md` has a tested backup/restore section covering both online and offline
  procedures, with the photo-directory caveat.
- The procedure is **actually tested** — back up a populated instance, destroy the `.db` and photo
  directory, restore, and verify contacts, notes, activities, reminders, relationships, and photos
  all survive.
- If a `make backup` target is built: test it against a running instance and confirm the output file
  restores cleanly.

## Landing note (2026-08-09)

Landed. `docs/deployment.md`'s Backups section now documents the tested online (`VACUUM INTO`) and
offline (stop → copy) procedures, the restore steps with a verification step, and the photo +
attachments directories that live outside the SQLite file. `getting-started.md`'s one-line Backup
blurb was corrected to point at it (it used to recommend copying the live `.db`, which WAL mode makes
unsafe).

`make backup` (backend/Makefile) produces a consistent online snapshot via
`database.BackupSnapshot` (checkpoint → `VACUUM INTO` → `PRAGMA integrity_check`), refusing to
overwrite an existing output, building at a temp name and moving into place atomically so a failed
run leaves no partial file and can never delete a file it didn't create. A `cmd/backup` CLI mirrors
the `cmd/migrate` shape: reads `SQLITE_DB_PATH`, output from a positional arg / `BACKUP_PATH` /
timestamped sibling.

**Two deliberate deviations from this ticket's suggestions, both for correctness in the actual
deployment:**

1. **`make backup` is a Go CLI, not a `sqlite3` shell-out.** The shipped image has no `sqlite3`
   binary (backend/Dockerfile's runtime deps), so the target could not have worked in the documented
   Docker deployment. `VACUUM INTO` is a plain SQL statement the app's own driver handles normally —
   the ticket's stated rationale for the target — so the CLI just runs it.
2. **The checkpoint is `wal_checkpoint(PASSIVE)`, not `wal_checkpoint(TRUNCATE)`.** The ticket's trap
   premise ("VACUUM INTO may not include uncheckpointed WAL frames") does not hold on the bundled
   SQLite (≥3.51.3; verified empirically that VACUUM INTO reads through the WAL), and TRUNCATE
   returns busy=1 the moment any connection holds a read transaction — which would make the "online
   backup" fail exactly when a live server is busy, the one situation it exists for. PASSIVE never
   returns busy; the checkpoint is kept as a best-effort tidy-up and VACUUM INTO is the actual
   correctness guarantee.

**Tests** (all hand-verified by breaking the code): real-migrated-DB Go tests for online capture of
recent WAL writes, the destroy-and-restore round trip, source-untouched, overwrite refusal,
no-litter-on-failure, default naming, and the real `make backup` target; plus
`frontend/e2e/backupRestore.spec.ts`, a Playwright e2e that builds the backend from source, seeds a
user + contact + note + activity + reminder + relationship + photo + attachment through the real API,
runs the real `make backup`, destroys the `.db` and both directories, restores, restarts, and
asserts every entity and every byte survived. The e2e CI job now provisions Go for that spec.

T26's cross-ticket note ("a restore should not resurrect rows pending purge — decide and document")
is answered in deployment.md's Restore section: a file-level restore is a deliberate point-in-time
rollback, and resurrecting soft-deleted (not-yet-purged) rows is inherent to it — documented as the
expected semantics, with no partial/merge restore.
