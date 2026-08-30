# Migration expectation files (`NNNNNN_name.expect.yaml`)

MIG-03 (issue #438) proves a migration **preserved meaning**, not just that it
ran. The semantic migration suite (`backend/internal/schemafixture`'s
`TestMigrationsPreserveSemanticContent`) snapshots every populated table's
content before and after migrating each release fixture (v0.6.0 → current),
compares per-contact canonical `Record`s under the TEST-03 semantic-equivalence
oracle, and asserts soft-deleted rows survive (`Unscoped`).

**The default is that nothing changes.** A migration that legitimately
transforms data must declare exactly what it transforms in a file next to its
SQL: `NNNNNN_name.expect.yaml` beside `NNNNNN_name.up.sql`. Absent file =
"asserts no change" — a silent rename without a backfill fails the suite naming
the emptied field.

## Format

```yaml
# 000046_street_suffix_split.expect.yaml
migration: "000046_street_suffix_split"   # MUST match the file's own name

# One entry per changed CELL. row is the rowid as stored; from/to are the
# concrete before/after values the migration performs.
changes:
  - table: contacts
    row: "1"
    column: firstname
    from: "Ada"
    to: "Augusta"
  - table: contact_sync_links
    row: "1"
    column: etag
    from: 'W/"fixture-etag-1"'
    to: ""
```

`table`/`row`/`column` identify the cell exactly as the suite's content
snapshot reads it (the fixture data is deterministic, so rowids are stable).
`from`/`to` are compared to the canonical cell rendering, so numeric and
boolean cells take their plain values and strings should be quoted.

## Guarantees (the file cannot rot)

The suite compares the declared changeset against the actual one in **both
directions**:

- a declared `from` that never matched the before-state fails — the migration
  stopped performing the change, or the fixture data moved;
- a declared `to` the migration no longer produces fails;
- an actual change the file does not declare fails — the file understates what
  the migration does.

## Scope

The default no-change assertion covers every column present in the *release*
schema: values, dropped columns (a rename without a backfill), added/removed
rows, and any backfill into a table the release did not have. Columns a
migration **adds** are new data and outside the guarantee — validate their
backfill with their own migration test. New empty tables are fine.

To author one: mirror a real case into a temporary file, run
`go test ./internal/schemafixture/ -run TestMigrationsPreserveSemanticContent`,
and confirm the declared change passes while each anti-rot direction above
fails. The loader's unit tests in `migration_expectation_test.go` are the
authoritative spec.
