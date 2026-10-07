# ADR 0035: Controller-to-service boundary and a declarative delete-cascade registry

- **Status:** accepted — slice 1 (contact delete) shipped with issue #1495; the lint ratchet and the
  user-scope registry are specified here and are follow-ups.
- **Date:** 2026-10-06
- **Implements:** issue #1495.
- **Related:** `docs/adrs/0004-soft-vs-hard-delete-semantics.md` (the soft/hard rule each registry step
  records), `docs/adrs/0012-canonical-database-invariants.md`, `docs/adrs/0015-temporal-semantics.md` (the
  one wall clock), CLAUDE.md backend traps #1, #6 and #7.

## Context

Two architectural choices make the backend's tests heavier and its recurring bug classes easier to
reintroduce.

1. **Controllers own persistence.** 79 of 84 controller files take a `*gorm.DB` directly. Every behaviour
   test is therefore an HTTP + real-DB test (correct per trap #1, but the `controllers` leg is the slowest in
   CI); DB-failure behaviour needs GORM-level fault injection; and business rules (merge, delete cascades,
   cadence, derived columns) sit beside request binding and get reused by copy.
2. **Manual cascade enumeration.** Soft delete never fires SQL `ON DELETE CASCADE` (trap #6), so
   `DeleteContact`, the bulk delete, `CommitContactMerge`, `deleteUserCascade` and the account-delete paths
   each enumerate dependent tables by hand. An audit once found 14 tables they had missed. The completeness
   tests (`controllers/delete_cascade_coverage_test.go`, `services/purge_completeness_test.go`) detect an
   omission only *after* someone wrote the model, and the same `Delete` meant different things per call site
   (the audit-hook FK bug, #1471) — exactly what trap #7 forbids.

## Decision

### 1. Business operations live in `services/`; controllers bind, authorize, call, map errors

New multi-row or business operations land in `services/` behind plain functions (or a small interface) that
take a `*gorm.DB` / transaction and an explicit time (`now time.Time`, or a `clock.Clock` once #1534 lands),
never reading the wall clock themselves. A controller binds the request, resolves and authorizes the
resource, calls the service, and maps its error to an `apperrors` response. Existing code is **not**
mass-migrated: a controller moves when it is next materially changed.

The boundary is enforced by a shrink-only ratchet, not by a rewrite: an analyzer
(`internal/lint/controllerdb`, modelled on `internal/lint/gormerr` and the coverage ratchet) counts
`db.Where/Create/Save/Delete/...` call sites in `controllers/` and fails when the count rises above a
committed baseline file. The count can only go down. *(Specified here; implemented as a follow-up — it does
not change runtime behaviour and needs its own baseline generation and pre-commit wiring.)*

### 2. A cascade is a declarative registry, not a hand-written function

A delete cascade is data: an ordered `[]CascadeStep` where each step names the dependent **table**, how to
build its **model**, how to **match** the deleted entity's rows (always `user_id`-scoped, trap #5), its
**mode**, and a recorded **reason**:

| Mode | Meaning | Rule |
|---|---|---|
| `soft` | user-authored content: tombstone for sync, something for undo | the model must carry `gorm.DeletedAt` |
| `hard` | edge-/join-shaped or system-generated rows | the model must **not** carry `DeletedAt` (or GORM would soft-delete it) |
| `mutate` | a row that outlives the entity (a pointer cleared, a credential revoked) | supplies its own `Apply` |

This is trap #7 made machine-checkable: soft-vs-hard is a property of the step's model, decided once, and a
test proves each step's declared mode agrees with the model.

**Slice 1 — contact delete.** `services/contact_delete.go` holds `ContactCascadeRegistry()` and
`DeleteContactAssociations` / `DeleteContactInTx` / `DeleteContact`. It is consumed by `DeleteContact`
(`controllers/contact_controller.go`), the bulk `delete` action (`bulk_operation_controller.go`) and
`CommitContactMerge`'s loser cleanup (`contact_merge_controller.go`), so the three paths cannot drift. The
step order and every statement are preserved from the hand-written function (reminders before life events;
notification deliveries before their soft-deleted reminders; the notes `updated_at` touch; the `users`
self-pointer clear; contact-feed revocation). The two wall-clock reads in the old function now arrive as a
`now` parameter.

**Completeness is derived from the migrated schema** (`services/contact_delete_test.go`): every table that
has an FK to `contacts`, or a contact-reference column (`contact_id`, `contact_vcard_uid`,
`member_vcard_uid`, `entity_id`, `source_id`, `target_id`, `uid_low`, `uid_high`,
`self_contact_vcard_uid`), must be a registry step or appear in a reasoned `contactCascadeNotRegistered`
map (today: `audit_events`, the append-only trail). Both directions are enforced, so a new model with a
`contact_id` and no registry entry fails CI, and a stale entry fails too. A cross-check in
`controllers/delete_cascade_coverage_test.go` pins the registry's delete steps to that file's independent
`go-cascade-contact` classification, and `contactSweepSoft` keeps pinning each table's soft/hard outcome
behaviourally. A per-step fault test hides each step's table in turn and requires the delete to fail and
roll back whole.

### 3. User scope is the same registry with a different target

`deleteUserCascade` (admin delete and self-service account delete, ADR 0020) enumerates user-scoped tables
with `Unscoped()` hard deletes — the trap #7 exception — and is deliberately **not** moved in slice 1: its
`SkipHooks` audit semantics and its dependence on the `users` FK `ON DELETE CASCADE` for a set of tables
(`fkCascadeUser`) make it a different shape of step list, and moving it together with the contact path would
mix two behaviour-preservation proofs in one change. The follow-up reuses `CascadeStep` with a
`UserCascadeTarget`, asserts `registry ⊇ every table with a user_id FK` the same way, and retires
`declaredCascadeCoverage`'s hand-kept `goCascadeUser` bucket.

### 4. Tests

Service-level tests run against `dbtest.New` (the real migrated schema, trap #1) with an injected `now`
instead of the wall clock. Controllers keep one HTTP test per status code plus the existing
`delete_cascade_*` sweeps untouched. The mutation scope follows the code: `contact_controller.go` leaves
`controllers-delete-cascade`, and a new `services-delete-cascade` scope targets `contact_delete.go`.

## Consequences

- Adding a contact-dependent table is a registry edit plus a test failure that says so, rather than a
  silent omission found by a later audit.
- `DeleteContact` behaviour (cascade order, soft/hard outcomes, audit rows, webhooks, status codes) is
  unchanged; the move is covered by the pre-existing sweeps, audit and rollback tests.
- The registry encodes policy in one place, so the soft-vs-hard reasoning is reviewable as data rather than
  inferred from 30 call sites.
- The ratchet and the user-scope registry are still owed; until they land, trap #6's "enumerate by hand"
  advice still applies to `deleteUserCascade`.
- Open PRs touching `contact_controller.go` (a DB-fault sweep that reshapes the delete transaction; the
  clock change that adds a time parameter to the cascade helper) will conflict with the removal of
  `deleteContactAssociations`; the resolution is to apply their change to `services/contact_delete.go`.
