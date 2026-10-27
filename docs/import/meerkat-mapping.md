# Meerkat import mapping (issue #353)

The import reads a Meerkat CRM SQLite database file directly (ADR 0007 — the only carrier of
Meerkat's relationship, circle, and custom-field structure). `backend/meerkat` opens it read-only
and tolerates any Meerkat schema version; `backend/services/meerkat_import.go` maps it.

The **web assistant** landed with issue #550: the user uploads the `.db` file from the Data
settings page, picks which source user to import (a Meerkat deployment can hold several; the
importer never silently mixes accounts), sees the loss report before committing, and confirms —
the import runs in the background so the flow survives navigating away. Endpoints:
`/api/v1/contacts/import/meerkat/*`; orchestration in `services.MeerkatImportManager`. The
uploaded file is held only as a `0600` temp file for the session's lifetime.

## Live API import (issue #2013)

The same wizard can instead **connect to a running Meerkat** (`POST /contacts/import/meerkat/connect`
→ the shared `fetch` / `status` / `preview` / `confirm` / `cancel`). The user enters the Meerkat URL
and either an API token (preferred) or a username and password. No file copy, no stopping Meerkat,
no WAL concerns — and, unlike the DB path, **profile photos transfer**.

```
URL + token|password → meerkatapi.Client (SSRF-guarded, no redirects, size/time limits)
  → CheckHealth (no credential sent yet) → login → Detect (feature probe) → Me → CountContacts
  → [fetch] FetchSnapshot (paged GET /contacts?includes=notes,activities,relationships,reminders)
  → the same meerkat.Snapshot → MapMeerkatSnapshot → preview/loss report → confirm
  → after the commit: GET /contacts/:id/profile_picture → photostore (validate + re-encode)
```

- **Version detection.** Meerkat's `/health` returns a hard-coded `"version": "0.1.0"`, so the
  generation is inferred from the shape of a contacts page (`meerkatapi.Detect`). One adapter,
  `v1.5-v1.7`, is supported: the data API is identical across v1.5.0–v1.7.0 (checked by diffing the
  tags). A server missing the v1.5 fields, or returning fields the adapter does not know, is
  `supported: false`; `connect` still opens the session and reports why (`reason_code`
  `meerkat_version_older_than_supported` | `meerkat_version_newer_than_tested` for clients to
  translate, `reason` as the English fallback), and `fetch` then requires
  `accept_untested: true` (the "Try anyway" confirmation). Nothing is ever written before the user
  has reviewed the preview and loss report. A new Meerkat release means one new adapter beside
  `meerkatapi/snapshot.go` plus its golden fixtures.
- **Scope.** Only the authenticated user's data (the API is already user-scoped), so there is no
  source-user step. The API hides `vcard_uid`/`etag`, so UIDs are not preserved (the file path
  keeps them); re-runs still match on the stable source ids (`contact/<id>`) and the preview flags
  every already-imported contact as a duplicate. Contact-less (unassigned) notes are not reached by
  this path.
- **Photos.** `contacts.photo` is no longer `unsupported` on this path: the bytes are downloaded
  after the import transaction commits and saved through `photostore` exactly like an upload. A
  failed download is counted (`photos_failed`) and never fails the import.
- **SSRF.** `MEERKAT_BLOCK_PRIVATE_URLS` (default `false`, restart-only), the same trade-off as
  `MONICA_BLOCK_PRIVATE_URLS`: a self-hosted Meerkat usually lives on a LAN or Docker network, so
  private addresses are allowed unless the operator opts into blocking (set it `true` for cloud /
  multi-tenant). There is no per-request override. Redirects are never followed.
- **Credentials** live only in the in-memory session: never persisted, logged or echoed; a
  password login is logged out of Meerkat when the import finishes, is cancelled, or the session
  expires. `connect` reports `insecure_transport` for plain `http://` to a non-local host.
- Not available on the embedded (Android) server, like the Monica import.

## Path (database file)

```
meerkat DB file → meerkat.Open (read-only, schema-tolerant)
  → MapMeerkatSnapshot → ImportSourcePlan (neutral Records + graph entities)
  → ExecuteSourceImport (one transaction, idempotent, per-record loss reporting)
```

By default the import targets the **first source user**; a multi-user deployment's other users'
data is never silently mixed in. Pass an explicit source user id to select one.

## Mapping

| Meerkat (source) | Lands as | Notes |
|---|---|---|
| `contacts.firstname` / `lastname` | `Card.Name` given / surname | |
| `contacts.middle_name` / `prefix` / `suffix` | `Card.Name` given2 / title / credential | |
| `contacts.nickname` | `Card.Nicknames[0]` | |
| `contacts.gender` | `CRMEnvelope.Gender` | free-text, normalized |
| `contacts.email` / `emails[]` | `Card.Emails` | JSON array wins, scalar fallback |
| `contacts.phone` / `phones[]` | `Card.Phones` | |
| `contacts.urls[]` / `impps[]` | `Card.Links` / `Card.ImppAddresses` | |
| `contacts.addresses[]` / `address` | `Card.Addresses` | structured wins; scalar → Full-only |
| `contacts.birthday` | `Card.Anniversaries[kind=birth]` | `YYYY-MM-DD` or `--MM-DD` |
| `contacts.anniversary` | `Card.Anniversaries[kind=wedding]` | |
| `contacts.organization` / `department` | `Card.Organizations` (+ unit) | |
| `contacts.job_title` / `role` | `Card.Titles` title / role | |
| `contacts.vcard_uid` | `Card.UID` (preserved) | stable cross-system identity |
| `contacts.vcard_extra` | `Passthrough.VCard` | best-effort, same legacy shape |
| `contacts.archived` | `Contact.Archived` | CRM-local flag |
| `contacts.how_we_met` | `CRMEnvelope.HowWeMet` | |
| `contacts.work_information` | `CRMEnvelope.WorkInformation` | |
| `contacts.contact_information` | `CRMEnvelope.ContactInformation` | |
| `contacts.circles[]` | `Circle` entities + members, and `CRMEnvelope.Circles` | one circle per unique name |
| `contacts.custom_fields{}` | `FieldDefinition` + `FieldValue` | one definition per key |
| `contacts.food_preference` | `Preference{category: food}` | |
| `relationships` (flat table) | `RelationshipEdge` | direction below |
| `notes` | `Note` | |
| `activities` + `activity_contacts` | `Activity` + attendees | |
| `reminders` | `Reminder` | recurrence vocabulary passthrough |
| soft-deleted rows (any table) | — (reported `skipped`) | tombstones are not content |
| `contacts.photo` | — (reported `unsupported`); **downloaded on the live-API path** | bytes live on the source server's filesystem; only the API can fetch them |

### Relationship direction

A Meerkat row `{contact_id: X, name: "John", type: "Father"}` is stored on X's page and renders
as "Father: John" — the **type describes the named person's role relative to the owner**. The
local edge stores the source's role relative to the target, so the edge is **named person →
owner** with the matched type verbatim: `"Father"` → `parent_of`, `"Daughter"` → `child_of`,
`"Spouse"` → `spouse_of`, and so on (via `models.MatchLegacyRelationType`; unrecognized values
fall back to `related_to`). Only one direction is stored; the inverse is derived. Real data may
contain both halves of a reciprocal pair (added from each contact's page); the two halves are
collapsed to one edge, since storing both would double-render the derived inverse.

## Known limitations

- **Photos** cannot transfer from the database file (only a filename is stored). Reported
  `unsupported`, never silent.
- **Dangling relationships** (a legacy `name` with no `related_contact_id`, or an id pointing at
  no contact row) have no target to edge to. Reported `unsupported` with the person's name.
- **Other users' data** is not imported by default (first source user only).
- **Very long values** are preserved verbatim (the local columns are unbounded TEXT; ADR-0002's
  "preserve, don't reject").

## Verify

`backend/services/meerkat_import_test.go` drives the checked-in fixture through the real pipeline
into a migrated schema and asserts every mapped field lands, direction is preserved, losses are
named, and a re-run does not duplicate. `TestMeerkatImport_RelationshipDirectionPinned` is the
hand-verify pin: inverting the edge mapping fails it.
