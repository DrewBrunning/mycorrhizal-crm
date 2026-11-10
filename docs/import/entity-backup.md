# Entities in the account bundle and CSV backup (issue #2300)

Reusable organisations, places and pets ([ADR 0043](../adrs/0043-reusable-entities.md)) travel in the
two owner-backup formats and nowhere else. This page is the shape reference and the list of what is
deliberately **not** carried.

## Where entities appear

| Surface | Entities | Notes |
|---|---|---|
| Account bundle (`GET /api/v1/export/account`, `mycorrhizal` import source) | **Yes**, full fidelity | Three additive plan collections, below |
| Flat CSV backup (`GET /api/v1/export`) | **Yes**, three trailing sections | Below; restore is `services.RestoreEntityCSV` |
| vCard 3/4, JSContact export, CardDAV, contact shares | Only the **projected** primary link's ORG/ADR value | No entity object, collection or `X-` property |
| vCard/JSContact/CSV-column import, CardDAV PUT | **Never** | Inbound ORG/ADR stay contact values; nothing is created, matched or rewritten |
| Monica / Meerkat import | **Never** | No mapping; `import_source_links` gains no entity kind for them |
| MCP | **Never** | `get_contact` reflects the projected ORG/ADR at the default sensitivity |

`backend/services/entity_write_paths_test.go` pins the "never" rows structurally: a file outside the
declared entity surfaces that mentions an entity model fails the build.

## Account bundle

`plan.entities`, `plan.entity_links` and `plan.entity_field_values` are additive to bundle `version: 1`.
A bundle written before 1.7 has none of the keys and imports as no entities (no issue is reported); the
exporter always writes all three as arrays. Live rows only. Nothing is filtered: `normal`, `private` and
`secret` rows all travel, with their sensitivity, because the bundle is the owner's own backup.

| Collection | Row |
|---|---|
| `entities` | `id` (UUID), `kind`, `name`, `sensitivity`, `core` (the kind's JSON: organisation `organization`/`url`/`notes`, place `address`/`notes`, pet `species`/`breed`/`birth`/`death`/`notes`), `created_at`, `updated_at` |
| `entity_links` | `id` (UUID), `contact_vcard_uid`, `entity_id`, `kind`, `role`, `role_label`, `is_primary`, `start`, `end` (partial dates), `sensitivity` |
| `entity_field_values` | `field_definition_id`, `entity_id`, `value` (raw JSON) |

The entity-target `custom_field_definitions` travel in the existing definitions collection. A value of
an entity-target definition is in `entity_field_values`, never in `custom_field_values` (which stays
contact-owned). Cross references use stable ids (`entities.id`, `contact_vcard_uid`), never uint keys.

**Restore.** Entities land through the same engine as every other bundle row, with its rules: ids are
re-minted and intra-bundle references are rewritten (issue #1355), `import_source_links` makes a re-run a
no-op, and every row is validated like a REST write (kind, name, sensitivity, the kind's `core`, the link
role vocabulary, that a value's definition targets the entity's kind). A bundle id the account already
owns (live or in the trash) is a per-row **conflict**: reported with category `skipped`, never
overwritten, never duplicated, and its dependents are reported as referencing an entity that was not
imported. A link whose contact is not in the bundle or an invalid row is dropped and named on the report.

## CSV backup

`ExportData` appends, after `=== REMINDER_COMPLETIONS ===`, in this order:

| Section | Columns |
|---|---|
| `=== ENTITIES ===` | `ID, Kind, Name, Sensitivity, Core (JSON), Notes, Created At, Updated At` |
| `=== ENTITY LINKS ===` | `ID, Contact ID, Contact Name, Entity ID, Entity Name, Kind, Role, Role Label, Primary, Start Date, End Date, Sensitivity, Contact UID` |
| `=== ENTITY FIELDS ===` | `Entity ID, Definition Key, Value` (raw JSON) |

`Contact UID` is appended to the ADR column list: the numeric `Contact ID` means nothing on another
instance, so restore resolves the link's contact by its stable vCard UID. `Notes` repeats the text already
inside `Core (JSON)` for readability; restore reads `Core (JSON)`. Every cell passes `csvSafe`, so a name,
note or role label that starts with `=`, `+`, `-`, `@`, tab or carriage return is prefixed with `'`.
Restore strips that marker again; the one ambiguity is a genuine value that itself begins with `'`
followed by one of those characters, which loses the apostrophe. Nothing is filtered by sensitivity.

The `=== CONTACTS ===` section is unchanged (no organisation column, and entity-target definitions add no
column), so a pre-1.7 export reads exactly as before.

**Restore** (`services.RestoreEntityCSV`) reads only the three entity sections and lands them through the
same pass as the bundle, so the same rules apply (re-run is a no-op, an owned id is a conflict, rows are
validated). The CSV carries no contacts and no field definitions, so a link needs its contact already in
the account (matched by `Contact UID`) and a value needs an entity-target definition with the same
`Definition Key` already present for the entity's kind; a missing one is reported, not invented.

## Named loss

Entities, their notes and custom fields, pet data, and a link's role, role label, dates and primary flag
have **no RFC 9553/9554/9555 home**, so no vCard or JSContact export carries them (ADR 0043 decision 3,
the same recorded loss as ADR 0025 periods). Only the projected ORG/ADR value of a *current primary* link
leaves the instance, filtered by sensitivity in the query. The bundle and the CSV are the only complete
copies.
