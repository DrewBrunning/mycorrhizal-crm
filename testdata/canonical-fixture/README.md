# Canonical pathological contact fixture (TEST-02, issue #430)

One reviewable, versioned JSON manifest — `manifest.json` — plus a Go loader
(`backend/internal/canonicalfixture`) that populates a **real migrated
database** from it. This is the single dataset that the migration (MIG-01/02/03,
#436/#437/#438), import/export (DATA-01..04, #441–#444), round-trip (TEST-03,
#431), merge-golden (TEST-05, #433) and performance (PERF-01, #468) suites are
expected to consume instead of inventing their own thin fixtures. PERF-01
builds *scale* on top of this dataset rather than a different one.

The arrangement deliberately copies `testdata/contract-fixtures/` (issue #257/
#266): one canonical copy, each consumer points its own loader at it. This
manifest is pure JSON so a TS/Kotlin loader can parse the same file later; the
Go loader is the first consumer.

## What the manifest covers

Every item on the milestone's Canonical Test Dataset list, plus the traps this
repo has actually shipped, so the fixture is a regression net and not a wish
list:

- **Card surface**: multiple names (extra `given2`/`title`/`credential`
  components, `sortAs`, `isOrdered`), nicknames, organizations and units,
  titles, emails, phones (with `features`/`contexts`/`pref`), IMPP /
  social profiles / other online services, structured addresses (incl. the
  T79 sub-street slots), anniversaries, photo media, keywords, relatedTo and
  members, localizations.
- **Traps, by name** (each is asserted in `canonicalfixture`'s own tests, so
  deleting any one of them fails a suite):
  - **trap #3** — fields with no flat-column home (`SpeakToAs`,
    `PersonalInfo`, `SocialProfiles`, `OtherOnlineServices`, extra name
    components, `Keywords`, `Notes`, `Members`, `Localizations`) must survive
    `Record → Contact → Record` via the neutral Card. The
    `RecordForContact` vs `RecordFromContact` bug class (CLAUDE.md trap #3) is
    exactly what drops these.
  - **Gender (#515)** — the documented canonical-field round-trip hole, now
    carried in the CRM envelope (`crm.gender`).
  - **sensitivity** — relationship edges, hobby preferences and vCard-projected
    custom fields at `normal`/`private`/`secret`, so the export-filtering rule
    ("filtered in the query, not in the caller") is exercised in both
    directions (default excludes, the T9 `IncludeSensitive` opt-in includes).
  - **soft-deleted rows alongside live ones** — the `gina` contact is created
    with a full cascade surface (notes, life events, gifts, preferences,
    custom field values, external identity, attachment, activity, edges,
    memberships, taggings) and then tombstoned exactly the way `DeleteContact`
    does: content soft-deleted, join/edge rows hard-deleted. `julie` then
    re-uses `gina`'s `vcard_uid`, which only succeeds because
    `idx_contacts_vcard_uid_user` is partial (`WHERE deleted_at IS NULL`) —
    the trap #7 unique-index behavior, load-bearing.
- **Content entities**: notes (one ~1700-char note, one soft-deleted), life
  events (year-only / month+day / full / edge-case dates, `remind`, related
  entities), gifts, activities (with `external_ref` and contacts), custom
  fields (text/string/number/multi-valued, one vCard-projected,
  `internal-only` and secret), households (family + roommates, members incl.
  a pet), circles, tags, preferences (incl. one soft-deleted), external
  identities, attachment metadata rows.
- **Data pathologies**: Unicode and non-Latin data (Japanese name components
  with phonetic readings, emoji, mixed scripts), empty/null values (a contact
  whose card is only a given name and uid), very long values (a ~1700-char
  note, a ~930-char `how_we_met` near the 1000-char validation ceiling), and
  edge-case dates (year-less leap-day birthday, far-future wedding, DST gift
  date), and deliberately duplicate/conflicting records (`hugo`/`ida` share
  email, phone, and a near-identical name — a real duplicate-detection pair).

## Consuming it

Go (the only loader today):

```go
db := dbtest.New(t)                          // or database.InitDB(...) — real migrated schema
m, err := canonicalfixture.Read()            // parses + validates the checked-in manifest
ds, err := canonicalfixture.Populate(db, m)  // loads it, returns the created Dataset
```

- `Read()` locates `testdata/canonical-fixture/manifest.json` by walking up
  from the working directory, so it works from `backend/` or the repo root.
- `Populate` runs in a single transaction and creates every contact through
  `models.ApplyRecordToContact` (trap #2 — never direct field mutation),
  scoped to one fresh user.
- `Dataset` keys contacts by their manifest `name` (`ds.Contacts["ada"]`) and
  holds every created row, tombstoned ones included.
- `ds.User`, `ds.Contacts`, `ds.Notes`, `ds.LifeEvents`, `ds.Gifts`,
  `ds.Relationships`, `ds.Households`, `ds.Circles`, `ds.Tags`,
  `ds.FieldDefinitions`, `ds.Preferences`, `ds.ExternalIdentities`,
  `ds.Attachments`, `ds.Activities`.

Extend it, don't fork it: a suite that needs a special case adds a row to the
shared manifest (or a new section — contacts, notes, life_events, gifts,
relationships, households, circles, tags, custom_fields, preferences,
external_identities, attachments, activities), never a private copy of the
file.

## The manifest schema

`version` must match `canonicalfixture.ManifestVersion` (currently `1`);
bumping the schema means bumping both, and that diff is the versioning record.

A contact entry embeds the neutral `contactmodel.Record` verbatim — `card` /
`crm` / `passthrough` use exactly the JSON keys the nested REST API and the
neutral model use — plus manifest-level fields:

| Field | Meaning |
|---|---|
| `name` | stable key; every other section references contacts by it |
| `comment` | why this entry exists (which trap / dataset item it pins) |
| `soft_deleted` | create, then tombstone like `DeleteContact` (phase A contact row, phase B dependent cascade) |
| `recreates_vcard_uid_of` | copy the named (earlier, soft-deleted) contact's `vcard_uid` — pins the partial unique index |

All other sections reference contacts by `name`; the loader resolves to
`vcard_uid`. Cross-references that do not resolve fail `Validate` naming the
section, so a broken manifest is caught before any database work.

## Rules

- **Reviewable diff.** The manifest is hand-authored and pretty-printed; a
  schema change is a normal, human-readable PR diff.
- **Versioned.** `version` at the top; a schema change that would silently
  reinterpret old data bumps it, and the loader rejects unknown versions.
- **Load-bearing.** `canonicalfixture`'s own tests assert every declared
  field round-trips and every trap is present. Deleting any single trap record
  fails them — that is what keeps downstream suites from silently starting to
  define their own datasets.
- **Metadata only for attachments.** The manifest stores `Attachment` rows
  (stored/original name, content type, size) but never file bytes — binary
  content has no home in a reviewable JSON fixture. Tests that need a physical
  file create it themselves.
- **Not for format correctness.** RFC-verbatim example cards live in
  `docs/golden-fixtures/` (ADR-0003); that oracle is not extended here.

## Verifying

```bash
cd backend && go test ./internal/canonicalfixture/
```

Run the full suite (`go test ./...`) to confirm nothing else regressed when
consumers start switching to this fixture.
