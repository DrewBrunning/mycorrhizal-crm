# ADR 0006: Monotonic per-row revision tokens

- **Status:** accepted
- **Date:** 2026-08-28
- **Depends on:** ADR 0004 (soft vs hard delete semantics)
- **Split from:** issue #456 (CON-01, optimistic concurrency) — this ADR and the schema carry the
  token; the conditional-write *enforcement* (If-Match/If-None-Match handling, `409`/`412`,
  CON-02/03/04) is #456's scope and deliberately **not** implemented here.

## Context

Optimistic concurrency needs a per-row token that a client can read and later echo back in a
conditional write, so the server can detect "someone else changed this row since you read it". Three
entities (`Contact`, `Activity`, `LifeEvent`) already carried a token for CardDAV/CalDAV sync — the
`etag` column, generated in `AfterCreate`/`AfterSave` as:

```go
c.ETag = fmt.Sprintf("e-%d-%d", c.ID, c.UpdatedAt.Unix())   // Contact (uint PK)
a.ETag = fmt.Sprintf("e-%d-%d", a.ID, a.UpdatedAt.Unix())   // Activity (uint PK)
l.ETag = fmt.Sprintf("e-%s-%d", l.ID, l.UpdatedAt.Unix())   // LifeEvent (UUID PK)
```

That token has two problems for optimistic concurrency:

1. **One-second resolution.** Two updates to the same row inside the same second produce an
   identical ETag — a conditional-write check built on it inherits the lost-update hole it exists to
   prevent.
2. **Not exposed to REST clients.** The field is `json:"-"` on all three, so only CardDAV reads it
   (`opts.IfMatch.MatchETag(contact.ETag)`, `carddav/backend.go`). A REST client cannot obtain the
   token it would need to send back.

## Decision

### The token

Every **user-authored, soft-delete entity** (ADR 0004's soft-delete class) carries a
**monotonic per-row integer revision counter** starting at 1, incremented on every persisted write:

- `Contact`, `Activity`, `LifeEvent` (which already had an `etag`)
- `Note`, `Reminder` (which had neither — they are the same soft-delete class and will need the
  token the moment a sync/merge surface touches them)

The counter is a real column (`revision INTEGER NOT NULL DEFAULT 1`), maintained in the model's
`AfterCreate`/`AfterSave` hooks (the same hook architecture that already maintains the `etag`). The
CardDAV/CalDAV `etag` is then **derived from the counter** — `e-{id}-{revision}`, keeping the
existing `e-{id}-{n}` shape for compatibility — never the reverse (no wall-clock input at all, so the
token is immune to clock granularity and skew).

Why a counter and not a timestamp: a counter is strictly monotonic per row, needs no clock, and its
only failure mode is the write path's own read-modify-write, which the `_txlock=immediate` DSN
(CLAUDE.md backend trap 9) serializes. A timestamp token is only as good as the clock's resolution
and the "did the second tick over" races that produced the bug above.

### Which entities are excluded, and why

The hard-delete edge/join rows (`RelationshipEdge`, `CircleMember`, `ContactTag`, `HouseholdMember`,
`ContactSyncLink`, `CalendarEventLink`, `FieldValue`) are **deliberately excluded**. Per ADR 0004 and
CLAUDE.md trap #7, they are small, bounded, re-pulled-wholesale rows with natural-key composite
unique indexes; a client re-fetches the collection rather than tracking each row's deaths, so a
per-row version token has no consumer. Writing the exclusion down here is the "say explicitly why
rather than silently omitting" discipline from the ticket.

### REST surface

`revision` is exposed **read-only as a response field** on every entity that carries it:

- `Contact`: on `ContactRecordResponse` and `ContactSummary` (the list shape) — the model field
  itself stays `json:"-"` like `ETag`, surfaced only through the DTOs.
- `Activity`, `LifeEvent`, `Note`, `Reminder`: on the model directly (`json:"revision"`), since the
  model *is* the response DTO for those endpoints.

The legacy top-level `etag` response field on `ContactRecordResponse` stays (CardDAV compatibility).
Adding a response field is additive/non-breaking under MAINT-02's draft criteria (issue #491) — it
does not change or remove any existing field, so this ADR does not wait on that ticket.

**Write-path enforcement is out of scope**: no conditional-check handling, no `409`/`412`, no
`If-Match`/`If-None-Match` parsing on the REST side. That is #456 (CON-01). The schema and the
read-side exposure exist now precisely so #456's write path can build on a real column, and so
TEST-02's canonical fixture (#430) can populate a real revision value rather than a placeholder.

### CardDAV behavior

CardDAV's existing `If-Match` conditional writes (`carddav/backend.go`, the one place conditional
writes are already honored) keep working unchanged: they compare against the `etag` column, which now
reads `e-{id}-{revision}` instead of `e-{id}-{unix}`. Same shape, different source — no adapter or
server change required.

## Migration strategy

A single up/down migration adds the columns and backfills:

- `revision INTEGER NOT NULL DEFAULT 1` on `contacts`, `activities`, `life_events`, `notes`,
  `reminders`.
- `etag TEXT` on `notes` and `reminders` (the two entities that never had one).
- Existing rows' `revision` is backfilled from their old `etag`'s numeric suffix when it parses
  (`e-{id}-{n}` → `revision = n`), so the counter starts **above** any historical token and no old
  ETag value can ever be reused (a reset-to-1 backfill would make `e-{id}-1234` eventually recur and
  trip a client holding the pre-migration `e-{id}-1234`). Rows with no parseable etag default to 1.

## Consequences

- Two writes to the same row inside the same second produce different revisions (the regression the
  ticket pins, tested against a real migrated schema — CLAUDE.md trap #1).
- A REST client can read a resource's revision from a normal response and use it later for #456's
  conditional writes.
- `Note`/`Reminder` gain both a revision and a CardDAV/CalDAV-style `etag` for the first time —
  purely additive; no existing behavior changes for them beyond the new fields.
- Every plain `Save`/`Updates` on a revision-bearing entity now issues one extra
  `UPDATE ... revision, etag` statement (bypassing hooks, no recursion). The `ID == 0` guard
  (bulk `Model(&T{}).Where(...)` updates) prevents the counter from being touched there, exactly as
  the existing etag guard does.
- Limitation, accepted for the schema half: the counter is read-modify-write on the in-memory value,
  so two *concurrent* last-writer-wins writers can stamp the same number (both read N, both write
  N+1). The row's token stays consistent with its own content and strictly monotonic across its own
  write history; serializing the increment atomically is #456's write-path concern, not this
  schema's.
