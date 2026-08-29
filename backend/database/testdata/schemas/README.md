# Versioned migration schema dumps (MIG-01, issue #436)

One **schema-only** SQL dump per release at or above the supported-upgrade
floor (`v0.6.0`, issue #529). These are the historical schemas the migration
(MIG-02, #437), automated-upgrade (DEPLOY-02, #451) and cross-version-backup
(BACKUP-01, #453) suites upgrade and read. The loader that consumes them is
`backend/internal/schemafixture`; the generator is `backend/cmd/genschema`.

| Release | Migration version |
|---|---|
| `v0.6.0.sql` | `000031` |
| `v0.6.1.sql` | `000036` |
| `v0.6.2.sql` | `000043` |
| `v0.6.3.sql` | `000044` |

## What a dump contains

The schema exactly as the release's highest applied migration left it:

- every non-internal schema object (`CREATE TABLE` / `CREATE VIRTUAL TABLE` /
  `CREATE INDEX` / `CREATE TRIGGER`) from `sqlite_master`, in dependency order.
  FTS5 shadow tables are omitted — the virtual table's own `CREATE` recreates
  them, and the FTS triggers (which share the `<table>_<suffix>` naming) are
  kept;
- the `schema_migrations` row for the version, so a fixture built from the
  dump presents the correct version and a clean (non-dirty) flag — a fixture
  that omitted it would test a different situation than the one that occurs in
  production.

## How each was produced

The migration chain is **linear, append-only, and frozen**: migrations
`000001..N` in the current tree are byte-identical to what the tagged release
shipped (verified during generation). Each dump is therefore produced by
applying migrations `000001..N` from the current embedded chain to a scratch
database and dumping it — equivalent to checking out the tag and dumping its
schema:

```sh
cd backend
go run ./cmd/genschema          # regenerates every dump in place
# or, for one release:
go run ./cmd/genschema 2>/dev/null   # the command regenerates all of them
make gen-schema-fixtures        # same, via the Makefile target
```

The output is deterministic (no timestamps), so regenerating and diffing is a
real check — `schemafixture.TestSchemaDumpsReproduceCurrentChain` does exactly
that in CI, and fails if a dump drifts from the current chain or an old
migration file is ever edited.

## Frozen

**A historical schema never changes retroactively.** These files are
committed, reviewed artifacts — never regenerated in place as part of a
feature's normal development. Regenerate them only when a **new release**
ships, as part of that release's fixture step:

1. add the release to `internal/schemafixture`'s `SupportedReleases`;
2. `cd backend && go run ./cmd/genschema`;
3. review the diff (the new release's dump appears; existing dumps must be
   byte-identical).

The completeness test (`TestEverySupportedReleaseHasADump`) and the
`docker-publish.yml` release gate both fail until the new dump exists, so a
release without a fixture cannot be shipped silently.

## Do not hand-edit

Every statement in a dump is generated. Hand-editing one creates a fixture
that disagrees with both the tag it claims to represent and the chain the
reproducibility test checks it against.
