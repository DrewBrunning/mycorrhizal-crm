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

Two startup states are enforced today; the third is MIG-04's scope (issue
#439), which will harden the dirty and ahead-of-binary cases further:

| State | Behavior (current) | Operator action |
|---|---|---|
| Sub-floor schema (below `000031`) | **Refuse**, print the two-step message above, exit | Two-step through `v0.6.0`, or the documented bridge |
| Dirty schema | Force the version and re-run from the next migration (the defined crash-recovery path, already exercised by the fault-injection suite) | Reboot; if it recurs, restore the pre-upgrade backup |
| Schema ahead of the binary | Boot with no pending migrations (MIG-04 will turn this into a refusal) | Deploy the newer version or restore a matching backup |

## How upgrades are tested

- **Schema fixtures (MIG-01, issue #436):** one committed schema-only dump per
  release at or above the floor lives in
  `backend/database/testdata/schemas/` (see that directory's README). Each is
  generated from the embedded migration chain (frozen and append-only) and
  populated at test time from the canonical TEST-02 manifest.
- **Chain upgrades:** `internal/schemafixture`'s upgrade tests run every
  adjacent hop (`v0.6.0 → v0.6.1 → … → current`) and the longest supported
  skip (`v0.6.0 → current`) against real migrated fixture databases, asserting
  row counts and search consistency survive. A new release without a fixture
  fails CI (the completeness test plus the docker-publish gate).
- **Full-stack upgrades (DEPLOY-02, issue #451):** the release-by-release
  Docker harness validates the whole install (database + photos + attachments)
  end to end against the same release set.

## Document consistency

The floor (`v0.6.0`, migration `000031`) is defined in
`backend/database/migrate.go` (`SupportedUpgradeFloorVersion` /
`SupportedUpgradeFloorTag`) and re-exported by `internal/schemafixture`.
`schemafixture.TestDocsStateTheFloor` asserts this document still names both,
so the published policy cannot drift from the code.
