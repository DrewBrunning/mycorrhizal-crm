# Monica import mapping (issue #351)

The import maps a `monica.Snapshot` — the complete in-memory copy of a Monica account in the
shape the live API fetch produces. `backend/monica` defines the wire types and the JSON loader;
`backend/services/monica_import.go` is the mapping (a deliberate port of the upstream Meerkat
assistant's proven mappers, meerkat-crm#211/#216/#218, re-targeted onto the neutral model and real
graph entities). The live client — pagination, rate limiting, SSRF-guarded transport, the
assistant UI — is the deferred #549 ticket.

## Path

```
monica.Snapshot (fixture or live fetch) → MapMonicaSnapshot → ImportSourcePlan
  → ExecuteSourceImport (one transaction, idempotent, per-record loss reporting)
```

## Mapping

| Monica (source) | Lands as | Notes |
|---|---|---|
| `first_name` / `last_name` | `Card.Name` given / surname | last-name-only contacts promote |
| `nickname` | `Card.Nicknames[0]` | |
| `gender_type` M/F/O | `CRMEnvelope.Gender` male/female/other | display name fallback |
| `information.dates.birthdate` | `Card.Anniversaries[kind=birth]` | `is_year_unknown` → `--MM-DD`; age-based dropped |
| `information.dates.deceased_date` (+ `is_dead`) | `Card.Anniversaries[kind=death]` | bare death flag without a date reported |
| `information.career.job` / `company` | `Card.Titles` title / `Card.Organizations` | |
| `information.how_you_met.general_information` | `CRMEnvelope.HowWeMet` | |
| `description` | `CRMEnvelope.ContactInformation` | |
| `food_preferences` | `Preference{category: food}` | |
| `tags[]` | `Circle` entities + members, and `CRMEnvelope.Circles` | grouping concept |
| `addresses[]` | `Card.Addresses` | |
| `contactFields[]` | `Card.Emails` / `Phones` / `Links` / `ImppAddresses` | routed by protocol/type |
| `information.avatar` (photo/gravatar) | carried on the plan | **not fetched by the mapping** — the assistant (#549) downloads |
| `is_starred` | `Contact.IsFavorite` | CRM-local flag |
| `is_partial` | — (reported `skipped`) | name-only stubs behind relationships |
| `relationships` | `RelationshipEdge` | direction below; reciprocal halves collapsed |
| `activities` | `Activity` | untitled → "Activity" |
| `notes` | `Note` | |
| `reminders` | `Reminder` | frequency folded to local vocabulary; past one-time dropped |
| `gifts` | `Gift` | `offered` → `given`; `amount` preserved as text in notes |
| `calls` | `Activity{type: call}` | the `InteractionTypeCall` home |
| `tasks` | `Note` | no entity home; dated note |
| `debts` | `Note` | no entity home; dated note |

### Relationship direction

Monica's own source (ApiRelationshipController + `Contact::setRelationship`) is the oracle: a row
`{contact_is: A, of_contact: B, type: "daughter"}` means **B is A's daughter** — the type
describes `of_contact`'s role relative to `contact_is`. The local edge stores the source's role
relative to the target, so the edge is **of_contact → contact_is** with the matched type verbatim
(`"daughter"` → `child_of`, `"spouse"` → `spouse_of`, `"colleague"` → `coworker_of`, ... via
`models.MatchLegacyRelationType`; unrecognized values fall back to `related_to`).

Monica writes **both directions** of every relationship (two rows, one with the reverse type). The
local graph derives the inverse from one stored edge, so importing both halves would render each
relationship twice; the mapper collapses the reciprocal pair — the lower Monica contact id's half
survives, a stable arbitrary choice.

### Reminder folding

`frequency_type`/`frequency_number` fold onto the local vocabulary: `one_time` → `once`, `week` →
`weekly`, `month`+3 → `quarterly`, `month`+6 → `six-months`, `month` → `monthly`, `year` →
`yearly`. A one-time reminder in the past is a dead row and is dropped. Recurring reminders are
scheduled at their first occurrence on/after today. Monica auto-creates a yearly birthday
reminder per contact; those keep `reoccur_from_completion = false` so marking one done never
walks the date forward a week each year.

## Known limitations

- **Avatars** are carried on the plan and downloaded by the assistant (#549); the backend mapping
  never makes network calls (reported `transformed`, never silent).
- **Tasks and debts** have no entity home and become dated notes; the mapping makes that explicit.
- **Gift amounts** arrive as free text (`"£34.50"`) and are preserved as text in the gift's notes
  rather than guessed at as `ValueCents`/`Currency` (the local pair must be set together and
  unambiguous).
- **Partial contacts** (name-only stubs Monica creates behind relationships) are not imported; the
  relationships that reference real contacts carry the meaning.
- **A bare `is_dead` without a date** has no neutral home and is reported, not guessed.

## Verify

`backend/services/monica_import_test.go` drives the checked-in fixture
(`testdata/monica-fixture/snapshot.json`) through the real pipeline into a migrated schema and
asserts every mapped field lands, direction is preserved, losses are named, and a re-run does not
duplicate. `TestMonicaImport_RelationshipDirectionPinned` is the hand-verify pin: inverting the
edge mapping fails it.
