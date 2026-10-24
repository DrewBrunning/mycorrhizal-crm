---
name: database-migration
description: Use when changing the database schema, a model's stored representation, a constraint, an index, or an upgrade path.
license: MIT
---

# Database migration

Outcome: an append-only migration pair, the matching model change, and a test
that proves existing production data survives. Read `backend/database/AGENTS.md`
and `backend/AGENTS.md` (traps #1 and #7) before writing SQL.

Prerequisites: the schema comes from hand-written SQL, never `AutoMigrate`. The
supported in-place floor is from `v1.0.0` (migration `000057`) and later; a
sub-floor database must still refuse to migrate.

## Steps

1. Add `NNNNNN_name.up.sql` and `NNNNNN_name.down.sql` under
   `backend/database/migrations/`. Never edit a shipped migration.
2. Update the GORM model. Add an explicit `gorm:"column:..."` tag for any
   acronym or unusual casing.
3. Register the model in `backend/models/schema_parity_test.go` so GORM columns
   and tag defaults stay equal to the migrated schema.
4. If a column is renamed/dropped/retyped, write the backfill and a
   data-preservation test beside `backend/database/migrate_datapreservation_test.go`.
5. If a new entity is added, register its contact cascade in
   `backend/services/contact_delete.go` (ADR 0035) and decide soft vs hard
   delete once, on the model (`docs/adrs/0004-soft-vs-hard-delete-semantics.md`).
6. Open the [migration test matrix](references/migration-test-matrix.md) before
   choosing tests: it maps the change class to the up/down shape, the
   data-preservation proof, and the release/schema-dump impact.
7. At release time, run the schema-dump generator and commit any diff with the
   registration in `backend/internal/schemafixture/releases.go`.

## DoD

```bash
cd backend && go test ./database/... ./internal/dbtest/...
cd backend && go run ./cmd/genschema
cd backend && go test ./internal/schemafixture/... ./cmd/genschema/...
```

The CI `upgrade-matrix` and `schemafixture-suite` jobs run the upgrade legs from
the committed per-release dumps. Do not claim a leg passed from a local run that
did not exercise it. Report any check you could not run; never imply it passed.

## Stop and ask

- The change would delete, retype, or reinterpret stored user data without a
  backfill or a recorded decision.
- A migration needs a second sub-floor exception.
- The down migration cannot restore the previous shape and the release process
  requires rollback.

Canonical detail: `backend/database/AGENTS.md`,
`docs/upgrade-compatibility.md`, `docs/development/testing.md` (Migration).
