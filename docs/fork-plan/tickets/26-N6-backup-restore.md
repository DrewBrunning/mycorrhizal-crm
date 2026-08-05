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
