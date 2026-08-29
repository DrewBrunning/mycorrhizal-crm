# Source imports — mapping reference

This directory documents the source-import mappings (ADR 0007, issues #351 and #353): what each
source system's data becomes here, and what it cannot carry. Read the table for the source you are
about to import *before* trusting the import.

## Shared behavior

Both mappers (Meerkat, Monica) feed one shared engine (`backend/services/import_source.go`) that:

- persists contacts through `ApplyRecordToContact` onto the neutral `Card`/`CRMEnvelope` model
  (RFC 9553/9554/9555), exactly like the REST API;
- writes the whole import in one transaction, so a failed record leaves nothing behind;
- reports every loss/degradation per record with its **record**, **field**, and **category**
  (`unsupported` = no home anywhere; `lossy` = degraded; `transformed` = mapped with a change;
  `invalid` = rejected; `skipped` = deliberate policy skip, e.g. a soft-deleted source row);
- records every produced row in `import_source_links` keyed by `(system, external_id, user_id)`,
  so **re-running the same import never duplicates**;
- scopes every read and write to the importing user.

## Sources

| Source | Path | Fixture | Mapping table | Known limitations |
|---|---|---|---|---|
| [Meerkat](meerkat-mapping.md) | direct read of the Meerkat SQLite database | `testdata/meerkat-fixture/` | [table](meerkat-mapping.md#mapping) | photos, dangling relationships, multi-user ambiguity |
| [Monica](monica-mapping.md) | a fetched `monica.Snapshot` (live client in #549) | `testdata/monica-fixture/` | [table](monica-mapping.md#mapping) | avatars deferred, tasks/debts → notes, reciprocal collapse |
