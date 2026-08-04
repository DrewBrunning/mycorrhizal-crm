# Working in this repo

Mycorrhizal CRM — a hard fork of [meerkat-crm](https://github.com/fbuchner/meerkat-crm) being rebuilt
into a *personal relationship OS*: a Go/Gin + SQLite backend with a React/TypeScript/MUI frontend,
CardDAV/CalDAV sync, and a neutral RFC 9553/9554/9555 contact model.

This file is the conventions and hard-won traps. **Read it before writing code here** — several of the
items below are recurring bug classes that have shipped broken more than once.

## Orientation

| Where | What |
|---|---|
| `docs/fork-plan/95-backlog-and-priorities.md` | **The ticket board — the live plan.** Read the board section; the Tier 0–6 sections below it are historical. |
| `docs/fork-plan/tickets/` | One file per ticket, self-contained enough to implement from. |
| `docs/fork-plan/91-envelope-data-model.md` | Entity specs with field tables. The detailed source. |
| `docs/fork-plan/92-delivery-roadmap.md` | WP scope. **Not** the execution order — the board is. |
| `docs/fork-plan/00`–`50` | Neutral model, adapters, correspondence, integration history. |
| `backend/` | Go. Gin + GORM + SQLite, raw-SQL migrations. |
| `frontend/` | React 18 + TypeScript + MUI + vitest + Playwright. |

**Real production data exists as of `v0.2.0-alpha-candidate`.** The user deployed that tag to their own
server (Docker) for real-world testing on 2026-08-04. Every commit before that tag was written under the
opposite assumption ("no production data exists yet, breaking changes are cheap") — that history is fine
as-is, but it no longer describes the present. From here forward:

- **Migrations must preserve existing data.** A column rename/drop/retype needs a real backfill or an
  explicit, deliberate call that the data it holds is safe to lose — not a silent clean removal. When in
  doubt, ask; don't default to "just drop it."
- **Soft-deleted rows are someone's undo button now, not test fixtures.** Be extra careful with anything
  touching `DeleteContact`/`DeleteUser`'s cascade lists or the eventual T26 retention/purge job — the
  purge window is a real decision about how long someone's actual data stays recoverable.
- **Breaking API/contract changes still happen** (this is pre-alpha software, not a stability promise) —
  but breaking *data* is a different, higher bar than breaking a request shape.
- Still no backwards-compatibility scaffolding "just in case" for its own sake — the bar moved from "data
  loss is free" to "data loss needs a reason and a heads-up," not to "never change the schema again."

## Commands

```bash
cd backend && go build ./... && go vet ./... && gofmt -l . && go test ./...
```

```bash
cd frontend && npx tsc --noEmit && npx vitest run
```

Migrations: the server runs every pending migration on startup (`database.InitDB`, from an embedded FS),
so `make migrate-up` is only needed to migrate a database without booting the app. Migration files are
**hand-written SQL up/down pairs** in `backend/database/migrations/` — this project does **not** use GORM
`AutoMigrate` for schema. Never add a column by editing a model struct alone.

`make migrate-down` rolls back **exactly one** migration and prompts before doing it. It is destructive
— it runs that migration's `.down.sql`. Both the CLI and the Makefile read `SQLITE_DB_PATH`, so they
always target the same database the server does; they used to hardcode `mycorrhizal.db` and roll back
*every* migration, which with the squashed baseline destroyed the whole schema.

Dev server: use the Browser/preview tooling with `.claude/launch.json`'s `frontend-dev`, never a raw
`npm start` in a shell. The backend needs `JWT_SECRET_KEY`, `PROFILE_PHOTO_DIR`, `SQLITE_DB_PATH`, and
`FRONTEND_URL` matching the frontend's actual port or CORS will fail.

## Workflow

- **One branch per concern.** `feature/<thing>`. Implement → verify → commit per concern → push → merge
  only once confirmed working → delete the branch → update the board.
- **Research → plan → approve → implement → verify.** Use plan mode for anything with real design
  decisions. This cadence is established and expected.
- **Never commit to `main` or merge without being asked.**
- **Hand-verify your tests.** Break the code, confirm the new test actually fails, restore. A test that
  has never failed has proven nothing. This has caught real bugs here repeatedly.
- Update `docs/fork-plan/95-backlog-and-priorities.md` when a ticket lands.

## Backend traps

These are real bugs that shipped, not hypotheticals.

1. **Test against the real migrated schema, not `AutoMigrate`.** GORM's column-name derivation silently
   disagrees with the hand-written migration SQL, and `AutoMigrate`-based tests cannot see it. Use
   `database.InitDB(filepath.Join(t.TempDir(), "x.db"))` for anything touching persistence.
   - `HouseholdMember.MemberVCardUID` → GORM derived `member_v_card_uid`, migration said
     `member_vcard_uid`. Caught by a real-DB test.
   - `ContactSyncLink.ETag` → GORM wrote `e_tag`; the column is `etag`. **Shipped broken**, would have
     silently killed CardDAV incremental sync. Add explicit `gorm:"column:..."` tags for anything with an
     acronym or unusual casing.

2. **Never set `Card`/`CRM` by direct field mutation before `Create`.** `BeforeSave` derives the flat
   denormalized columns from the nested model; mutating the struct field directly skips it and your data
   silently doesn't persist. Use `ApplyRecordToContact`. This bit WP-81 and WP-83 the same way.

3. **`RecordForContact`, not `RecordFromContact`.** The former reads what is actually persisted
   (including data with no flat-field home — `SpeakToAs`, `PersonalInfo`, projections); the latter
   rebuilds from flat fields and **silently drops** that data. Using the wrong one was a live bug found
   across three call sites.

4. **Check `.Error` on every `db.Updates`/`db.Save`.** Three sites silently swallowed failures until
   audited.

5. **Ownership scoping is not optional.** Every handler scopes by `user_id` (or `Contact.VCardUID` for
   WP-80+ graph entities). There are zero IDOR holes today — keep it that way.

6. **Cascade deletes are manual.** Soft delete does not fire SQL `CASCADE`. `DeleteContact` and
   `DeleteUser` enumerate every dependent table explicitly — if you add an entity, add it there. Use
   `contact_controller.go`'s `DeleteContact` as the canonical checklist.

   Note `admin_user_controller_test.go`'s `assertGone` helper counts with `db.Model(...).Count()`, which
   **excludes soft-deleted rows** — so it passes whether a row is gone or merely marked. If you need the
   distinction pinned, assert with `Unscoped()`.

7. **Soft vs hard delete is a property of the model, not of the call site.** Decide it once when you
   create the entity; never make `tx.Delete(x)` mean different things in different functions.
   - **Content the user authored** (`Contact`, `Note`, `Activity`, `Reminder`, `LifeEvent`) →
     **soft delete**. Gives sync a free tombstone and undo something to work with.
   - **Edge- and join-shaped rows** (`RelationshipEdge`, `CircleMember`, `ContactTag`,
     `HouseholdMember`, `ContactSyncLink`, `CalendarEventLink`, `FieldValue`) → **hard delete**, per
     `RelationshipEdge`'s own doc comment. Small and bounded, so a client re-pulls them rather than
     tracking their deaths.

   **The rule is not arbitrary — check your unique constraints.** A soft-deleted row still occupies every
   unique index it is in, so a lingering dead one blocks re-creating the same key. Every table with a
   natural-key composite unique index hard-deletes for exactly this reason (a join row *is* its
   endpoints). If your new entity has a natural key, that is a strong signal it should hard-delete. Where
   an entity must soft-delete *and* carry a unique key, make the index partial —
   `... WHERE deleted_at IS NULL` — the way `idx_contacts_vcard_uid_user` does.

   **Operation-based variance was considered and rejected** ("cascades hard, single deletes soft"): it
   makes every future cascade site a chance to forget an `Unscoped()`, and the failure is silent. Tier 3c
   item 1 found 14 tables `DeleteUser`/`DeleteContact` had already missed. See the T26 ticket.

   `gorm.Model` gives soft delete for free, **but only works on uint-PK entities**. The UUID-string-PK
   entities have their own explicit `ID`/`CreatedAt`/`UpdatedAt`, so embedding `gorm.Model` there adds a
   conflicting `ID uint` and breaks them. Add the one field instead:
   `DeletedAt gorm.DeletedAt \`gorm:"index" json:"-"\``.

   **`DeleteUser` is the single deliberate exception** — it hard-deletes via `Unscoped()`, because
   `users.email`/`username` are unique and a soft-deleted account would block re-registration forever.
   If you find yourself adding a second exception, re-read this section first.

8. **`gorm.DB.Transaction` returns the closure's error verbatim**, so you can return a typed
   `*apperrors.AppError` from inside and type-assert it after to preserve a 404/400 instead of
   flattening to 500. `relationship_edge_controller.go` does this.

9. **`busy_timeout` alone does not stop `SQLITE_BUSY`; the DSN also needs `_txlock=immediate`.**
   SQLite does **not** invoke the busy handler when a *deferred* transaction upgrades its read lock to
   a write lock — it fails instantly instead, no matter how long the timeout. GORM wraps even a single
   `Create` in an implicit transaction, so two concurrent writes produced a 500 `database is locked` in
   under 5ms. `openDSN` sets `_txlock=immediate` so transactions take the write lock up front, which
   *is* a case the busy handler retries; WAL keeps readers unaffected. Pinned by
   `database/concurrent_write_test.go`. Don't remove the flag.

### Backend conventions

- Controllers: follow `circle_controller.go` / `life_event_controller.go` (the newer idiom) over older
  ones. `currentUserID(c)`, `middleware.GetValidated[T]`, `apperrors.AbortWithError`,
  `GetPaginationParams`.
- Join rows (membership) get **real nested sub-resource endpoints** (`POST/DELETE /circles/:id/members`),
  not a bulk-replace field. A duplicate add is a checked `409 ErrAlreadyExists`, not a sniffed constraint
  error.
- UUID-PK entities (`RelationshipEdge`, `Household`, `Circle`, `Tag`, `LifeEvent`, `FieldValue`) generate
  their ID in `BeforeCreate`. Everything older uses `gorm.Model`'s uint PK.
- Validation lives in struct tags + `middleware.ValidateJSONMiddleware`; custom validators
  (`phone`, `birthday`, `safeurl`, `relation_type`) are registered in `middleware/`.
- Sensitivity (`normal|private|secret`, `91.13`): anything above `normal` is excluded from exports and
  external sync **in the query**, not in the caller.

## Frontend traps

1. **vitest here has no auto-cleanup and no `globals: true`.** Add `afterEach(cleanup)` explicitly in
   component test files or you get "multiple elements found" failures.
2. **MUI appends `" *"` to a required field's accessible label.** `getByLabelText('Name')` fails;
   `getByLabelText('Name *')` works.
3. **Do not nest a `<Chip>` (renders `<div>`) inside `<Typography variant="body2">` (renders `<p>`).**
   Invalid HTML; React warns. Put both in a sibling flex `Box`.
4. **Frontend enum/registry lists are hardcoded mirrors of backend `oneof` validators.** There is no
   dynamic type-list endpoint anywhere in this codebase, by design. If you add a token backend-side, the
   frontend copy must be updated by hand — add a comment noting it must stay in sync.
5. **All five locale files get real translations** (`en`, `de`, `es`, `fr`, `it`), not English
   placeholders. `src/i18n/locales.test.ts` now enforces this: identical key sets in both directions,
   identical interpolation placeholders, and no *namespace* left byte-identical to English. That last
   check exists because an entire 25-key block once shipped as untranslated English in all four
   non-English locales. It cannot assert per-key difference — proper nouns and cognates are legitimately
   identical in bulk.
6. **Translate to a leaf key, never to an object node.** `t('contacts.personalInfo.kindOptions')` where
   `kindOptions` is a parent of `{expertise, hobby, …}` renders i18next's diagnostic —
   `key '…' returned an object instead of string` — as visible UI text. This shipped on the contact
   create/edit form in all five languages.
7. **Playwright's `e2e/global-setup.ts` hardcodes `http://localhost:7300`** for both the app origin and
   its direct API calls, separately from `playwright.config.ts`'s `baseURL`. The e2e suite cannot run
   against a different port without editing shared test infra.
8. **A Go struct field that is `omitempty` and a TS field that is required is a crash waiting to
   happen.** `db.Find` leaves a slice nil when nothing matches, `omitempty` then drops the key
   entirely, and a required TS type means nobody guards the `.length`. This took the whole prep view
   into the ErrorBoundary for any contact with no history. Collection fields on a response DTO should
   not carry `omitempty`, and a test asserting it must read the **raw JSON** — decoding into the Go
   struct makes "absent" and `[]` indistinguishable, which is exactly why the existing test passed.

### Frontend conventions

- The contact model is nested (`Card`/`CRMEnvelope`/`Passthrough` via `ContactRecordResponse`). The flat
  `Contact` type survives **only** for the list endpoint (genuinely flat on the wire) and for
  `MultiValueField`/`AddressFields`' editing contract. Do not reintroduce a flat adapter.
- API modules live in `src/api/<entity>.ts`, hooks in `src/hooks/use<Entity>.ts`, and dialogs/lists in
  `src/components/`. `relationshipEdges.ts` + `useRelationshipEdges.ts` + `RelationshipEdgeDialog/List`
  are the most recent, most complete example of the full pattern — copy that shape.

## Domain notes worth knowing

- **`RelationshipEdge.Type` describes the *source's* role relative to the target.** `type: "parent_of"`
  from A to B means "A is B's parent." Only one direction is ever stored; the inverse is derived from
  `models/relationship_type_registry.go`, never persisted. Creating from a contact's page sends
  `target_id: <viewed contact>`, so a dropdown label always describes the *other* party.
- **Only `status: confirmed` edges are fact.** `suggested` edges (household-inferred) must never be
  projected to standards, graphed, or treated as real outside a review surface.
- **Cadence resets on a *qualifying interaction*, not on completing a task** (`91.10`).
  `Activity.Qualifying()` exists for this and has had no consumer yet.
- **CardDAV/REST writes are full-overwrite by design.** `reconcileContactSync` intentionally discards
  local edits on remote change — documented, pinned by a test. Do **not** copy that policy into new
  two-way sync paths without deciding deliberately (see the T13 ticket).
- The three exporters (`vcard3`, `vcard4`, `jscontact`) all consume the same neutral `Card`, so filtering
  the `Record` *before* it reaches an exporter applies to all three at once.

## Security posture

A full security review landed (14 findings, all patched — see `95`'s Tier 1). Keep it: parameterized SQL
only, no `os/exec`, templates from an embedded FS, `user_id` scoping everywhere, explicit field
allowlists on updates (no mass assignment), CSV values neutralized against formula injection, SSRF guards
enforced in the transport dialer. Go toolchain is pinned; don't float it.

Known and accepted: no 2FA yet (ticketed as N8).
