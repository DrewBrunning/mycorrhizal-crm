# Backend — Go, Gin, GORM, SQLite

Read the root `AGENTS.md` first for the dev loop, safety boundaries, and implementation workflow.
This file is the backend-specific contract: conventions and the recurring bug classes below have all
shipped broken at least once. Test against the real migrated schema, never `AutoMigrate`.

## Backend traps

1. <a id="trap-1"></a>**Test against the real migrated schema, not `AutoMigrate`.** GORM's
   column-name derivation silently disagrees with the hand-written migration SQL, and
   `AutoMigrate`-based tests cannot see it. Use `dbtest.New(t)` (the real migrated schema) for
   anything touching persistence — including the shared `setupRouter(t)` controller helper.
   `internal/dbtest/automigrate_ratchet_test.go` fails on any new `AutoMigrate`/
   `Migrator().CreateTable` call; for a "table missing" error path use `dbtest.HideTable`.
   `models/schema_parity_test.go` pins every model's GORM columns **and** tag defaults to the
   migrated schema — a new model must be added to its registry (a completeness scan enforces it).
   Any acronym/unusual casing needs an explicit `gorm:"column:..."` tag: `HouseholdMember.MemberVCardUID`
   derived `member_v_card_uid` (migration said `member_vcard_uid`), and `ContactSyncLink.ETag`
   derived `e_tag` (column is `etag`) — that one shipped broken and would have killed CardDAV
   incremental sync. `gorm:"default:true"` / non-zero numeric defaults on plain `bool`/`int` fields
   make an explicit `false`/`0` write the default instead (issue #1240); the parity test rejects
   them outside a reviewed allowlist.

2. <a id="trap-2"></a>**Never set `Card`/`CRM` by direct field mutation before `Create`.**
   `BeforeSave` derives the flat denormalized columns from the nested model; mutating the struct
   field directly skips it and your data silently doesn't persist. Use `ApplyRecordToContact`.

3. <a id="trap-3"></a>**`RecordForContact`, not `RecordFromContact`.** The former reads what is
   actually persisted (including data with no flat-field home — `SpeakToAs`, `PersonalInfo`,
   projections); the latter rebuilds from flat fields and **silently drops** that data. A new
   plain-save path must not re-introduce a straight `db.Save` on a loaded contact without going
   through `models/contact_card_merge.go`'s merge. `mergeMedia` there deliberately keeps a
   `Card.Media{kind:"photo"}` entry whose URI is a remote `http(s)` URL when the flat `contacts.photo`
   is empty — that is a transient reference the ingest caller downloads later (ADR 0012 INV-D8),
   and dropping it is data loss. A `data:`-URI photo with no flat photo is still dropped (a real delete).

4. <a id="trap-4"></a>**Check `.Error` on every `db.Updates`/`db.Save`.** GORM returns `*gorm.DB`,
   not `error`, so errcheck cannot see a discarded result. `cd backend && go run ./cmd/gormerrcheck ./...`
   (`internal/lint/gormerr`) enforces it in CI and the pre-commit hook.

5. <a id="trap-5"></a>**Ownership scoping is not optional.** Every handler scopes by `user_id` (or
   `Contact.VCardUID` for graph entities). There are zero IDOR holes today — keep it that way.

6. <a id="trap-6"></a>**Cascade deletes are manual.** Soft delete does not fire SQL `CASCADE`.
   `DeleteContact` and `DeleteUser` enumerate every dependent table explicitly — if you add an
   entity, add it there. The contact side is the declarative `services.ContactCascadeRegistry`
   (`services/contact_delete.go`, ADR 0035): add a `CascadeStep` (table, soft/hard, reason) and the
   completeness test fails until any table referencing a contact is registered. `DeleteUser`'s
   `deleteUserCascade` is still hand-written. Assert with `Unscoped()` when you need to distinguish
   hard-deleted from merely soft-deleted rows — `db.Model(...).Count()` excludes soft-deleted rows.

7. <a id="trap-7"></a>**Soft vs hard delete is a property of the model, not the call site.** Decide
   once when you create the entity. **Content the user authored** (`Contact`, `Note`, `Activity`,
   `Reminder`, `LifeEvent`) soft-deletes; **edge/join-shaped rows** (`RelationshipEdge`,
   `CircleMember`, `ContactTag`, `HouseholdMember`, `ContactSyncLink`, `CalendarEventLink`,
   `FieldValue`) hard-delete. A soft-deleted row still occupies every unique index, so every
   natural-key table hard-deletes; where an entity must soft-delete *and* carry a unique key, make
   the index partial (`... WHERE deleted_at IS NULL`, as `idx_contacts_vcard_uid_user` does). Do not
   vary this by operation — the failure is silent. `gorm.Model` only works on uint-PK entities; for
   UUID-PK entities add `DeletedAt gorm.DeletedAt \`gorm:"index" json:"-"\`` instead of embedding it.
   **`DeleteUser` is the single deliberate exception** — it hard-deletes via `Unscoped()` so the
   unique `users.email`/`username` can be reused. Re-read this section before adding a second one.

8. <a id="trap-8"></a>**`gorm.DB.Transaction` returns the closure's error verbatim**, so you can
   return a typed `*apperrors.AppError` from inside and type-assert it after to preserve a 404/400
   instead of flattening to 500. `relationship_edge_controller.go` does this.

9. <a id="trap-9"></a>**`busy_timeout` alone does not stop `SQLITE_BUSY`; the DSN also needs
   `_txlock=immediate`.** SQLite does not invoke the busy handler when a deferred transaction
   upgrades its read lock to a write lock — it fails instantly. `openDSN` takes the write lock up
   front; WAL keeps readers unaffected. Pinned by `database/concurrent_write_test.go`. Don't remove
   the flag.

10. <a id="trap-10"></a>**Never call `time.Now()` (or `time.Since`/`time.Until`) in
    `controllers`, `services` or `middleware` — read the injected clock** (issue #1494;
    `backend/internal/clock`). Handlers/middleware: `clock.FromContext(c).Now()` (`routes.go` installs
    `clock.System{}`; tests pre-install a `clock.Fake` with `clock.Install`). Services:
    `services.Now()`, swapped in tests with `defer services.SetClock(fake)()`. Rate limiters carry
    their own `SetClock`. An elapsed-duration measurement, a `net.Conn` deadline or a PRNG seed may
    stay raw with `// rawtime:allow <reason>`; bulk exceptions go in `internal/lint/rawtime/allowlist.go`
    with a reason and exact count. `go run ./cmd/rawtimecheck ./...` enforces it. JWT `exp`/`iat`/`nbf`
    are validated against the clock too (`middleware.ValidateTimeClaims`).

## Conventions

- Controllers: follow `circle_controller.go` / `life_event_controller.go` (the newer idiom) over
  older ones. `currentUserID(c)`, `middleware.GetValidated[T]`, `apperrors.AbortWithError`,
  `GetPaginationParams`.
- Join rows get **real nested sub-resource endpoints** (`POST/DELETE /circles/:id/members`), not a
  bulk-replace field. A duplicate add is a checked `409 ErrAlreadyExists`.
- UUID-PK entities (`RelationshipEdge`, `Household`, `Circle`, `Tag`, `LifeEvent`, `FieldValue`)
  generate their ID in `BeforeCreate`. Everything older uses `gorm.Model`'s uint PK.
- Validation lives in struct tags + `middleware.ValidateJSONMiddleware`; custom validators (`phone`,
  `birthday`, `safeurl`, `relation_type`) are registered in `middleware/`.
- Response DTO collection fields must not carry `omitempty`: GORM leaves a slice nil when nothing
  matches, the key disappears, and a required TS type crashes the client (frontend trap #8).
  `internal/omitemptyguard` enforces it; a test asserting this must read the **raw JSON**.
- **Sensitivity** (`normal|private|secret`): anything above `normal` is excluded from external sync,
  contact shares, and the neutral-`Card` exports (vCard 3/4, JSContact), filtered **in the query**
  and re-includable only via `?include_sensitive=true`. It is about copies that leave the instance,
  not an access-control tier against the owner. Three deliberate full-fidelity exceptions, each
  pinned by a test: the flat CSV export (`GET /api/v1/export`, "the user's own backup"), the account
  bundle (`GET /api/v1/export/account`, moving data between the user's own instances), and the
  contact map (`GET /api/v1/contacts/map`, the owner's own in-app view). Filtering any of them is a
  policy change, not a bug fix.
- **Domain notes:** a `RelationshipEdge.Type` describes the source's role relative to the target
  (`parent_of` from A to B means A is B's parent); only one direction is stored, the inverse is
  derived from `models/relationship_type_registry.go`. Only `status: confirmed` edges are fact —
  `suggested` edges are never projected, graphed, or treated as real outside a review surface.
  Cadence resets on a *qualifying interaction* (`Activity.Qualifying()`), not on completing a task.
  CardDAV/REST writes are full-overwrite by design (`reconcileContactSync` discards local edits on
  remote change); do not copy that policy into a new two-way sync path without deciding deliberately.
  The three exporters (`vcard3`, `vcard4`, `jscontact`) consume the same neutral `Card`, so filtering
  the `Record` before it reaches an exporter applies to all three at once.

## Generated artifacts — regenerate, never hand-edit

| Artifact | Regenerate with | Drift test |
|---|---|---|
| `testdata/contract-fixtures/` | `go run ./cmd/gencontract` | `contract_fixtures_test.go` |
| `frontend/src/generated/openapi.ts` | `go run ./cmd/gentsapi` | `tsapi_types_test.go` + `tsc` |
| `internal/apibaseline/testdata/v1.json` | `go run ./cmd/genapibaseline` | `internal/apibaseline` drift |
| `docs/data-01-field-compatibility-matrix.md` | `go run ./cmd/gencompatmatrix` | `correspondence/matrix_test.go` |
| `docs/int-01-integration-classification-matrix.md` | `go run ./cmd/genintegrationmatrix` | `integrations/matrix_test.go` |
| `database/testdata/schemas/` | `go run ./cmd/genschema` | schema-fixture completeness |
| `.gremlins/*.yaml` | `go run ./cmd/genmutationscope` | `internal/mutationscope` drift |
| Third-party notices | `go run ./cmd/gennotices` | notices drift |
| Config reference | `go run ./cmd/genconfigreference` | config-reference drift |

A backend response-shape change is: edit the schema/example in `backend/openapi.yaml` → regenerate
contract fixtures + TS types → fix whatever `tsc` flags. The API baseline diff is the review; a
removal there is the declared breaking change (`docs/breaking-change-policy.md`).

Performance/mutation pointers: `internal/perfbench` baselines are generated by `go run ./cmd/perfbench`;
budgets (`budgets.json`) are hand-authored and governed by `docs/development/perf-budgets.md`.
Mutation scopes and thresholds live in `internal/mutationscope.Scopes` — the single source for
`go-mutation.yml` and the generated configs; do not fork it. A new scope needs its matrix leg added
by hand (`TestWorkflowMatrixMatchesScopes` enforces parity).
