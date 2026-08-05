# T18 — WP-93 event history / audit trail

| | |
|---|---|
| **Rating** | 2 — additive and safe, but see the cost below |
| **Size** | L |
| **Depends on** | [T17](17-T17-change-feeds.md) |
| **Alpha** | after |
| **Source** | `92.5` |

## What it is

An immutable create/update/delete/merge/restore log across the entities, feeding three consumers per
`92.5`: **undo**, **sync**, and **debugging**. Also extends webhook event coverage to the new entities.

## The cost of deferring — worth understanding before agreeing to it

Technically this is pure-additive: one new append-only table, nothing existing changes shape. That is why
it sits post-alpha.

But an audit log **only knows what happened after you switch it on**. Everything from the alpha period is
unrecoverable — no undo, no "what changed this record," no debugging history for exactly the period when
you are most likely to want it, because that is when the app is newest and least trustworthy.

That is a judgment call about how much you value alpha-period history, not a migration hazard. If undo
matters to you during alpha, this belongs earlier.

## Design decisions — locked 2026-08-04

1. **Capture mechanism: GORM hooks (`AfterCreate`/`AfterUpdate`/`AfterDelete`).** Completeness is
   the property that matters for an audit log, and hooks guarantee every GORM write to a registered
   model is captured without relying on individual call sites remembering. Opacity at the call site
   is a feature here, not a bug.

2. **Transaction semantics: fire-and-forget from within the hook.** The audit write must not
   participate in the real write's transaction — an audit failure rolling back a contact save is
   unacceptable. Write the audit row in a separate goroutine with its own error logging. Trade:
   a write that succeeds but whose audit row fails to persist leaves a gap. Accept that gap over
   the alternative (audit failures blocking real writes).

3. **Undo scope: updates only.** Undo reverses the `before` snapshot from the audit event.
   Delete undo requires reconstructing cascade-deleted rows across ~14 tables and is a follow-up
   ticket if wanted. The "Done when" section reflects this narrower scope. The undo affordance
   must visibly reflect the T26 purge window — after `AUDIT_RETENTION_DAYS` passes, the audit
   row is gone and there is nothing to undo. Show a "revertable until" date, not a dead button.

4. **Retention: 90 days default, configurable via `AUDIT_RETENTION_DAYS`.** Purge job reuses T26's
   existing job-locked cron pattern. 90 days is short enough to control SQLite file growth during
   early real use (when writes are most frequent and a bug is most likely), long enough for the
   debugging use case. Document the volume trade: longer retention is linear row growth.

## What to build

1. **An append-only `audit_events` table** — entity type, entity ID, operation (`create|update|delete`),
   actor (user ID), timestamp, and a `before` snapshot (JSON) for update/delete events. Immutable:
   no update or delete path, ever. Model-level guard (no `Update`/`Delete` receiver methods) plus
   a DB-level safety net (`REVOKE INSERT, UPDATE, DELETE` or an application-level check). Hard-delete
   (system-generated, not user-authored — per T26).

2. **Capture via GORM hooks.** Register `AfterCreate`/`AfterUpdate`/`AfterDelete` on audited models.
   Each hook:
   - Builds the audit event struct.
   - Launches a goroutine to persist it (separate `gorm.DB` session, not the hook's transaction).
   - Logs errors but does not return them — per the transaction decision above.
   - Never captures `Password`, `TOTPSecret`, `ApiTokenHash`, or any `secret`-sensitivity field.
     Maintain a deny-list of field names checked in each hook.

3. **Retention purge.** A job-locked cron (reusing T26's `acquireJobLock` pattern) that deletes
   audit rows older than `AUDIT_RETENTION_DAYS`. Run alongside the existing T26 purge job.
   Initial implementation: default 90 days, configurable.

4. **Undo (updates only).** An endpoint that accepts an audit-event ID, loads the `before` snapshot,
   applies it via `ApplyRecordToContact` (or entity-specific equivalent), and writes back. Must
   verify the current `user_id` scope matches the audit event's. Must reject undo of events older
   than retention. Must reject undo of `delete`-type events (updates only).

5. **Webhook event coverage** for the newer entities (Circles, Tags, LifeEvents, Households, Gifts,
   etc.), reusing `services/webhook_service.go`'s existing delivery infrastructure.

## Traps

- **Volume.** Every write generating a row with a full snapshot grows fast on a single-file SQLite DB.
  The 90-day retention cap limits this, but an instance with heavy write patterns should be monitored.
- **Do not log secrets.** Password hashes, TOTP secrets ([N8](25-N8-2fa.md)), API token hashes, and OIDC
  tokens must never reach the audit table. Maintain a deny-list of field names checked in each hook.
  An audit log is a secondary copy of everything — treat it with the same care as the primary.
- **Sensitivity (`91.13`)** applies to audit rows too: a `secret` relationship's content should not become
  readable via the audit surface.
- **Goroutine lifecycle.** Audit writes fire from a goroutine detached from the hook's transaction.
  The goroutine opens its own `gorm.DB` session. Error logging is the only feedback mechanism — there
  is no synchronous confirmation that the audit row persisted. Test this explicitly.
- **Migration/backfill hooks.** A backfill script or migration that bulk-writes rows fires hooks.
  Either make hooks idempotent (they are — audit rows are append-only) or skip them during migration
  runs with `db.Session(&gorm.Session{SkipHooks: true})`. Decide in implementation.
- **Undo across the purge boundary.** The undo endpoint must reject events older than `AUDIT_RETENTION_DAYS`.
  After purge, the audit row is gone. Show the user a "revertable until" date in the UI so a dead
  button doesn't surprise them.

## Done when

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` green.
- Tests prove: every entity's create/update/delete produces exactly one event; the table rejects mutation;
  no secret-bearing field ever appears in an event; undo restores an updated record correctly; undo of a
  deleted record is rejected (not silently skipped); undo of a purged event (past retention) is rejected;
  a hook's goroutine failure does not roll back the real write.
- Hand-verified: remove one capture point (delete a hook registration), confirm the completeness test
  fails, restore.
- Retention purge job tested: rows older than `AUDIT_RETENTION_DAYS` are removed, newer rows survive.
- Volume measured against a realistic write pattern, with the retention decision verified to keep the
  SQLite file within acceptable bounds.

### Post-alpha note
This ticket is post-alpha — real production data exists. Changes that modify schemas or data must be additive and non-destructive. Migration files must be hand-written SQL up/down pairs. Test against `database.InitDB`, not `AutoMigrate`. For integrations: SSRF protection via `httputil.SafeDialContext` is mandatory for any outbound requests.

## Flash implementation notes

### Files to read first
- `/CLAUDE.md` at repo root (conventions, recurring traps, commands)
- Study an existing fully-implemented feature for the pattern: model → controller → routes → api → hooks → dialog → list → page wiring → i18n
- Common pattern references: `circle_controller.go` + test (newer idiom), `api/relationshipEdges.ts` + hook, `RelationshipEdgeDialog.tsx` + test, the `ContactInformation.tsx` tab + `ContactDetailPage.tsx` wiring

### Tests you must write before considering it done
- Backend: controller tests covering CRUD, ownership scoping, error states (not found, cross-user, 409 duplicate)
- Backend: real-DB test (`database.InitDB`, not `AutoMigrate`) for the core round-trip + any migration-dependent behavior
- Frontend: component test (`afterEach(cleanup)`, mock `fetch` with `vi.stubGlobal`) for dialog and list
- Hand-verify EVERY new test: break the code, confirm the test fails, restore. A test that has never failed has proven nothing.

### Self-verification checklist
1. `npx tsc --noEmit` — clean
2. `npx vitest run` — green (ALL tests, not just yours)
3. `cd backend && go build ./... && go vet ./... && gofmt -l . && go test ./...` — green
4. New migrations: run `make migrate-up` to verify they apply cleanly
5. All 5 locale files (`de/es/fr/it/en`) — real translations for any new strings

### Common traps (beyond CLAUDE.md)
- `gorm.Model` only works on uint-PK entities — UUID PK models need explicit `ID`/`CreatedAt`/`UpdatedAt` fields
- Backend tests use `setupRouter()` from `activity_controller_test.go` (sets `db`, `userID`, `cfg` in Gin context, uses AutoMigrate)
- Frontend component tests: `afterEach(cleanup)` mandatory; MUI appends `" *"` to required field `getByLabelText`
- Migration files: hand-written SQL up/down pairs — never add a column by editing the struct alone
- `gorm:"column:xxx"` tag is mandatory for acronyms/compound words — GORM silently derives wrong names
- New entities: decide soft vs hard delete per T26's rule (user-authored content → soft, edge/join rows → hard)
- Delete cascade: add new entities to `deleteContactAssociations` in `contact_controller.go` and `DeleteUser` in `admin_user_controller.go`
