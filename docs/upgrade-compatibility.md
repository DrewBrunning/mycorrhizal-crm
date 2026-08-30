---
title: Upgrade Compatibility
nav_order: 12
---

# Upgrade compatibility

**This is the canonical supported-upgrade statement (issue #529).** The upgrade
test matrix (MIG-01 fixtures in `backend/internal/schemafixture`, DEPLOY-02's
automated harness) and the startup migration gate (`database.RunMigrations`) are
all defined against this document; a change to the policy below is a change to
those, and vice versa. This page publishes the decided policy; it does not
re-derive it.

## The supported range

**In-place upgrade is supported from `v0.6.0` and later.**

- `v0.6.0` is the floor. It is the last release before the hardening series
  began, and every release at or above it is covered by a committed schema
  fixture (see "How upgrades are tested" below).
- **Version-skipping within the range is supported.** `v0.6.0 → current`
  directly must work, not only `v0.6.0 → v0.6.1 → …`. Self-hosted operators
  skip versions routinely.
- **Post-`1.0`:** any `1.x` upgrade from any earlier `1.x`, and from the final
  `0.9.x`. The floor moves only at a major version.
- **Downgrade is unsupported.** Rolling back means installing the previous
  version and restoring the pre-upgrade backup (see below).

### Upgrading

```sh
# 1. Back up (the server does NOT take a pre-migration backup for you yet —
#    see docs/deployment.md → Backups; the automatic mandatory backup ships
#    with DEPLOY-02/issue #530).
docker compose pull
docker compose up -d
```

Database migrations run automatically on startup. Upgrading is unattended; that
is exactly why the supported range is bounded and why the refusal cases below
fail loudly instead of best-effort.

## What happens below the floor

A database whose schema predates `v0.6.0` **refuses to migrate**. This is a
deliberate, loud refusal — never a partial migration, never a crash, never a
best-effort single hop:

- The server prints a message naming `v0.6.0` as the required intermediate and
  exits (the `Failed to initialize database` fatal log line).
- `cmd/migrate up` prints the same message and exits nonzero.
- Nothing is written to the database; the version is untouched and the database
  is left clean.

The exact message:

```
database schema version 30 predates the supported upgrade floor (v0.6.0, migration 31).
In-place upgrade is supported only from v0.6.0 and later; this version refuses to migrate a pre-floor database.
Upgrade this instance to v0.6.0 first, then run this version again — see docs/upgrade-compatibility.md.
```

### Two-step upgrade for a pre-floor instance

1. Back up the database and the file directories (`docs/deployment.md` →
   Backups — the three-piece backup: database, `PROFILE_PHOTO_DIR`,
   `ATTACHMENTS_DIR`).
2. Deploy the `v0.6.0` release (image `ghcr.io/<org>/mycorrhizal-crm:v0.6.0` or
   equivalent). Its startup migrations move the database to the `v0.6.0` schema
   (`000031`).
3. Verify it boots and serves.
4. Deploy the current release. It now sees a database at or above the floor and
   continues normally.

This is a **documented two-step, not a supported single hop**: only `v0.6.0`
(and later) is guaranteed to read the pre-floor schema and preserve its data.

## The one-time `v0.2.0-alpha-candidate` bridge

There is exactly one known sub-floor installation: the maintainer's own,
deployed at `v0.2.0-alpha-candidate` (2026-08-04). It gets a **one-time,
documented bridge**, not a standing support promise:

1. Back up everything (database, `PROFILE_PHOTO_DIR`, `ATTACHMENTS_DIR`; see
   `docs/deployment.md`).
2. Copy the database and run the bridge against the **copy** first:
   `MYCORRHIZAL_ALLOW_SUB_FLOOR_MIGRATION=1 make migrate-up` (or set the same
   env var when booting the current server against the copy). This env var is
   the only code-level way a pre-floor database can be migrated in one binary;
   it is deliberate, logged, and only for this bridge. A second exception
   would be a policy change, not a config option.
3. Verify the copy (boot it, log in, search, export; check the audit trail).
4. If the copy is clean, run the same step against the real database.

The bridge exists because the two-step path above depends on producing the
`v0.6.0` binary's exact startup migration run against a real database; if that
cannot be reproduced, the env var is the fallback. It is exercised in CI by
`database.TestFullChainMigrationPreservesRealData` (which asserts the refusal,
then runs the full chain through the override and asserts the seed data
survived).

## Refusal states

Three startup states are enforced (MIG-04, issue #439), all **fail-closed**: the
server refuses to start, logs at error level, and names the condition and its
recovery. A message that names the condition but not the remedy sends the
operator to the source; these all state both. In no case does any
configuration setting turn a refusal into a warning — the one exception is the
documented one-time `v0.2.0-alpha-candidate` bridge above, which is a policy
exception for a single known sub-floor deployment, not a bypass knob. The
step-by-step recovery for all three states — including exactly which pre-upgrade
backup file to restore and how long it must be kept — is
`docs/operations/migration-recovery.md`.

| State | Behavior | Operator action |
|---|---|---|
| Sub-floor schema (below `000031`) | **Refuse**, print the two-step message above, exit | Two-step through `v0.6.0`, or the documented bridge — see the [below-the-floor section](../operations/migration-recovery.md#below-the-floor) |
| Dirty schema | **Refuse** (`ErrDirtyMigration`): a migration started and did not finish, so the schema state is unknown | Restore the pre-migration backup and start again — see the [dirty-schema section](../operations/migration-recovery.md#dirty-schema). Only after verifying the schema actually matches the named version, `make migrate-force` (prompted, operator-only) — never automatic |
| Schema ahead of the binary | **Refuse** (`ErrSchemaAheadOfBinary`): the database knows migrations this binary does not, meaning a rollback is in progress | Deploy a binary that knows the newer migration, or restore the backup taken before the newer release ran — see the [ahead-of-the-binary section](../operations/migration-recovery.md#schema-ahead-of-the-binary) |

### Dirty schema — interrupted migration

The `schema_migrations.dirty` flag is set when a migration **starts and does
not finish**: the process was killed, the container was OOM-killed, the host
lost power, or the SQL itself failed partway. The flag exists to say "the
schema is in an unknown, partially-applied state; a human must look at it."

Previously the server force-cleared the flag at every boot and migrated on top
of the torn schema — presenting a half-applied database as healthy (issue
#546). It does **not** do that anymore. On a dirty database the server refuses
to start with a message naming the dirty version, what it means, and the
recovery:

```
database is in a dirty migration state at version 43: a migration started and did not finish, so the schema may be only partially applied and does not match any known version. Refusing to start (fail-closed). Restore the pre-migration backup and start again — see docs/deployment.md (Backups → Restore). If you have verified the schema actually matches version 43, the operator-only escape hatch is `make migrate-force` (or `go run cmd/migrate force`), which prompts for explicit confirmation; it is never applied on the startup path.
```

**Primary recovery: restore the pre-migration backup.** The schema state is
unknown by definition; the backup is the only state you can trust. Restore
per `docs/deployment.md` → Restore, then start again — the restored snapshot
is clean and migrations run normally.

**Operator-only escape hatch: `make migrate-force`.** Each individual
migration runs inside one transaction (SQLite DDL is transactional), so an
interrupted migration's DDL rolls back cleanly rather than landing torn — the
schema is either fully the previous version or fully the interrupted version,
never somewhere between. The recovery is therefore: verify which of those two
the schema actually matches, then clear the dirty flag at that version and let
the pending migrations run. `migrate-force` does exactly this for the **current
dirty version**, and it **prompts** (`Type 'yes' to continue:`) before doing
anything — it cannot be triggered non-interactively by accident, and it is the
only path that can clear the flag. The startup path has no equivalent.

### Schema ahead of the binary — bad rollback in progress

A database whose applied version is *higher* than this binary's newest
migration means the database was migrated by a newer release and this binary
has been rolled back. Downgrade is unsupported (see "The supported range"), so
starting anyway would have the binary misread columns it does not know about —
the same silent-corruption class as the dirty-force bug above. The server
refuses to start with `ErrSchemaAheadOfBinary`, naming both versions and the
recovery:

```
database schema version 45 is ahead of this binary (latest known migration 44): the database was migrated by a newer release and this binary has been rolled back. Downgrade is unsupported, so refusing to start. Deploy a binary that knows migration 45 (or newer) and start again, or restore the backup taken before the newer release ran — see docs/deployment.md (Backups → Restore).
```

### How each refusal is surfaced

Each refusal is its own typed error (`ErrSubFloorMigration`,
`ErrDirtyMigration`, `ErrSchemaAheadOfBinary`) so health/readiness and the
diagnostics run can report *which* state an install is in rather than a
generic boot failure, and so the migrate CLI prints the same message. The
server start path logs it via `logger.Fatal` (`Failed to initialize
database`); nothing is written to the database by any refusal.

## How upgrades are tested

- **Schema fixtures (MIG-01, issue #436):** one committed schema-only dump per
  release at or above the floor lives in
  `backend/database/testdata/schemas/` (see that directory's README). Each is
  generated from the embedded migration chain (frozen and append-only) and
  populated at test time from the canonical TEST-02 manifest.
- **Chain upgrades (MIG-02, issue #437):** `internal/schemafixture`'s upgrade
  tests run every adjacent hop (`v0.6.0 → v0.6.1 → … → current`) and the
  longest supported skip (`v0.6.0 → current`) against real migrated fixture
  databases, asserting row counts and search consistency survive. The
  migration-tests CI workflow matrixes one job per supported release
  (`v0.6.0 → current`, `v0.6.1 → current`, …), each migrating its fixture
  through `database.InitDB` (the path the server boots through) and asserting
  the final version and row counts, plus a down-direction job that round-trips
  every migration up → down → up against a populated fixture and gates on every
  migration shipping its `.down.sql`. A new release without a fixture fails CI
  (the completeness test plus the docker-publish gate).
- **Full-stack upgrades (DEPLOY-02, issue #451):** the release-by-release
  Docker harness validates the whole install (database + photos + attachments)
  end to end against the same release set.
- **Large datasets (issue #495):** the same chain-upgrade path is also tested
  against databases populated at 134x the canonical manifest (2,010 contacts,
  pathological records included) — every supported release migrates to the
  current schema with row counts and integrity intact, and the measured
  resource requirements (duration / peak memory / peak disk per path) are
  recorded in `docs/development/scale-testing.md`. Disk exhaustion during a
  large migration is asserted to fail closed in the chaos job.

## Document consistency

The floor (`v0.6.0`, migration `000031`) is defined in
`backend/database/migrate.go` (`SupportedUpgradeFloorVersion` /
`SupportedUpgradeFloorTag`) and re-exported by `internal/schemafixture`.
`schemafixture.TestDocsStateTheFloor` asserts this document still names both,
so the published policy cannot drift from the code.
