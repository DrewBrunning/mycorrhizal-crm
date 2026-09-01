# ADR 0008: REST conditional-write enforcement (If-Match / optimistic concurrency)

- **Status:** accepted
- **Date:** 2026-09-01
- **Depends on:** ADR 0006 (monotonic per-row revision tokens — the schema half)
- **Implements:** issue #456 (CON-01, optimistic concurrency) — the write-path enforcement half
  that ADR 0006 deliberately left out.
- **Hands off to:** #457 (CON-02 stale-write tests), #458 (CON-03 merge/conflict semantics),
  #459 (CON-04 idempotency).

## Context

ADR 0006 gave every user-authored soft-delete entity (`Contact`, `Activity`, `LifeEvent`, `Note`,
`Reminder`) a monotonic per-row `revision` counter, bumped in the model hooks on every persisted
write, and exposed it read-only on the wire (`revision` on the entity's response DTO). It stopped
there on purpose: no write-path checks, so a REST client could read the token but the server never
consulted one.

This ADR adds the enforcement: a client that read revision *N* can now ask the server to reject its
write if the row has moved past *N* since — the lost-update guard the token exists for. Two contract
questions the ticket calls out, both settled here:

1. **How does the client supply the token — a header or a body field?**
2. **What does a mismatch return — `409` or `412`?**

## Decision

### 1. The token travels in the `If-Match` request header, carrying the `revision` integer

`PUT` / `DELETE` on a revision-bearing entity accepts an `If-Match` header whose value is the
decimal `revision` the client last saw:

```
If-Match: "7"
```

- Quotes are optional (`If-Match: 7` is accepted); an optional weak prefix `W/` is tolerated.
- `If-Match: *` matches any existing row — it passes once the handler has loaded the row.
- A comma-separated list matches if **any** member matches.
- The value is the `revision` **integer**, not the CardDAV `etag` string (`e-{id}-{revision}`). The
  two tokens are derived from the same counter but the REST surface standardised on the bare
  integer — it is what `ContactRecordResponse.revision` / `Activity.revision` / … already put on the
  wire, and it sidesteps a client having to reconstruct or parse the `e-{id}-{n}` shape.

Header, not body field, because:

- It is the RFC 9110 §13.1.1 mechanism for exactly this ("perform the method only if the
  representation has not changed"), so HTTP caches, proxies, and client libraries already understand
  it. CORS already allows it (`main.go`).
- It keeps the precondition out of the entity body, so the same request body that a `GET` returned
  can be `PUT` straight back without stripping a server-managed field, and a body field named
  `revision` stays unambiguously read-only.
- CardDAV already does conditional writes this way (`carddav/backend.go`), so the codebase has one
  conditional-write idiom, not two.

### 2. A mismatch returns `412 Precondition Failed`

Code `PRECONDITION_FAILED`, standard `Error` envelope, `details.expected_revision` carrying the
row's current revision so the client can decide whether to re-fetch or force. The row is **not
modified** — the check runs after the row is loaded and before any mutation or transaction.

`412`, not `409`, because the rejection is a *precondition* the client explicitly attached to the
request (`If-Match`), which is precisely what `412` means in RFC 9110 §15.5.13 — and it is the
status CardDAV's `If-Match` path already returns (`webdav.NewHTTPError(http.StatusPreconditionFailed,
…)`). `409 Conflict` is reserved for a request that conflicts with the resource's state on its own
terms (e.g. a duplicate membership add → `ErrAlreadyExists`), independent of any client-supplied
precondition. CON-03 (#458) may still use `409` for a *content* merge conflict it chooses to surface
rather than reject; that is a different code for a different situation and does not change this one.

### 3. Enforcement is opt-in

A request with **no** `If-Match` header writes unconditionally — identical to behaviour before this
ticket (last-writer-wins). Reasons:

- Real production data and three independently-updating clients (web, Android, CardDAV) exist as of
  `v0.2.0-alpha-candidate`. Making `If-Match` mandatory would break every client that has not been
  taught to send it, on the next write.
- Under MAINT-02 (#491) adding an *optional* request header and a new response status is **additive**
  — it removes/narrows nothing — so it does not spend the breaking-change budget. Making the header
  *required* would be a removal (of the unconditional-write capability) and a different decision, out
  of scope here.
- The milestone criterion ("multiple clients cannot silently overwrite") is met for any client that
  opts in; tightening the default to reject tokenless writes, if wanted, is a deliberate later
  breaking change with its own ticket, not a side effect of this one.

### 4. Scope: `PUT` and `DELETE` on the five revision-bearing entities

`If-Match` is honoured on:

| Entity     | Endpoints                              |
|------------|----------------------------------------|
| Contact    | `PUT /contacts/{id}`, `DELETE /contacts/{id}`       |
| Note       | `PUT /notes/{id}`, `DELETE /notes/{id}`             |
| Activity   | `PUT /activities/{id}`, `DELETE /activities/{id}`   |
| LifeEvent  | `PUT /life-events/{id}`, `DELETE /life-events/{id}` |
| Reminder   | `PUT /reminders/{id}`, `DELETE /reminders/{id}`     |

`DELETE` is included because a stale delete (client removes a row it last saw at revision *N*, unaware
someone has since edited it) is a lost update too.

**Deliberately excluded**, matching ADR 0006's discipline of writing exclusions down:

- **State-toggle endpoints** — `POST /contacts/{id}/archive` · `/unarchive` · `/favorite` ·
  `/unfavorite`, `POST /reminders/{id}/complete`. Each flips a single independent flag or appends a
  completion; they carry no body to conflict over, they are effectively idempotent, and
  last-writer-wins on one boolean is not a lost update in the sense this ADR addresses. If a future
  need appears, they can adopt the same helper.
- **The hard-delete edge/join entities** (`RelationshipEdge`, `CircleMember`, `ContactTag`,
  `HouseholdMember`, `ContactSyncLink`, `CalendarEventLink`, `FieldValue`) — no `revision` column
  per ADR 0006 §"Which entities are excluded", so nothing to check.
- **`POST` creates** — no prior revision to be stale against. Retry-safety for creates is CON-04
  (#459), a separate mechanism (idempotency keys / natural-key dedup).

### 5. CardDAV is untouched

`carddav/backend.go`'s `PutAddressObject` keeps its own `If-Match` check against the `etag` **string**
column (`opts.IfMatch.MatchETag(contact.ETag)`), still returning `412`. ADR 0006 already re-pointed
that `etag` at the revision counter (`e-{id}-{revision}`), so the value moved but the contract did
not. The REST helper added here (`controllers.checkIfMatch`) is a separate code path and does not run
for CardDAV requests.

## Consequences

- A REST client that reads `revision`, then sends `If-Match: "<that value>"` on its next `PUT`/`DELETE`,
  gets `412` (row unchanged) if anyone — another REST client, a CardDAV sync, the Android app — wrote
  the row in between, and `200`/success otherwise. The success response carries the new `revision`.
- Two writes to the same row within the same wall-clock second are now distinguishable: the revision
  counter has no clock input, so the second writer's `If-Match` against the pre-first-write revision
  fails. This is the concrete bug the old `etag`-from-`UpdatedAt.Unix()` token could not catch.
- A tokenless client is exactly as safe (or unsafe) as before — no regression, no new protection.
- `checkIfMatch` is one shared helper (`backend/controllers/helpers.go`); a new revision-bearing
  write handler that forgets to call it silently has no protection. CON-02 (#457) is where a
  router-derived test makes that omission fail CI, the way #371 does for authorization.
- Concurrency limitation inherited from ADR 0006: two genuinely simultaneous last-writer-wins writers
  can still both read revision *N* and both stamp *N+1* (the increment is read-modify-write on the
  in-memory value, not an atomic `UPDATE … SET revision = revision + 1`). `_txlock=immediate`
  serialises the transactions so the row never ends up internally inconsistent, but the token is not
  a mutex. Closing that window (atomic increment + `UPDATE … WHERE revision = ?` compare-and-swap in
  one statement) is a follow-up if the race proves real in practice; the opt-in `If-Match` check
  above already covers the overwhelmingly common read-edit-write-later case.
