# T113 — Duplicate scan pairs a contact with itself ("merge_id must differ from keep_id")

| | |
|---|---|
| **Platform** | Backend |
| **Rating** | 4 — shows a bogus "duplicate" row and makes its Merge button fail with a confusing validation error |
| **Size** | S — one SQL keyword + a regression test |
| **Depends on** | Nothing |
| **Status** | **DONE** (2026-08-15) |
| **Source** | Testing note: *"Bulk merge shows duplicates that are already merged with an error 'merge_id must differ from keep_id'."* |

## Why this exists

`FindDuplicatePairs` (`backend/services/duplicate_service.go`) groups contacts per duplicate tier and pairs up
every distinct-id pair within a group. The phone tier (`:111-126`) tokenizes `phones_normalized` via a
`json_each` split and keeps tokens shared by more than one contact:

```sql
WITH split AS (
  SELECT contacts.id, value AS token
  FROM contacts, json_each('["' || replace(phones_normalized, ' ', '","') || '"]')
  WHERE user_id = ? AND deleted_at IS NULL AND phones_normalized != ''
    AND json_valid('["' || replace(phones_normalized, ' ', '","') || '"]')
    AND length(value) >= 7
)
SELECT split.id, token AS key, '' AS key2 FROM split
WHERE token IN (SELECT token FROM split GROUP BY token HAVING COUNT(*) > 1)
```

The email and name tiers can never self-pair because `email`/`firstname`/`lastname` are single-valued. The
phone tier can, because `phones_normalized` is **multi-token per contact** and a single contact can emit the
same token more than once:

- `FlattenPhones` (`backend/models/contact.go:256-269`) appends each phone's full digit string *and* its
  `PhoneKey` (last-10 digits) when they differ.
- Two of a contact's own numbers that reduce to the same `PhoneKey` — e.g. `+1 800 555 1234` (digits
  `18005551234`, key `8005551234`) alongside `800-555-1234` (digits `8005551234`, key `8005551234`) — produce
  `phones_normalized = "18005551234 8005551234 8005551234"`. The `split` CTE then emits the token
  `8005551234` **twice for the same id**, `COUNT(*) > 1` is satisfied by that one contact alone, and the
  group for the token is `[id, id]`.

`addTier` (`:132-157`) then pairs those two rows: `a == b`, `if a > b` is skipped, and the pair key becomes
`{A: id, B: id}`. The result loop (`:191-217`) happily resolves both sides to the same `ContactSummary` and
emits a `DuplicatePair` whose `A` and `B` are the same contact. The web review surface renders that as one
person listed against themselves; clicking Merge opens pair mode with `keeper === loser`, and the commit's
`keeper.ID === loser.ID` trips `loadMergePair`'s guard (`backend/controllers/contact_merge_controller.go:41-43`)
— `"merge_id: must differ from keep_id"`.

## What to build

Make the phone tier emit each `(contact, token)` pair at most once, so a contact can never join a token group
with a second copy of itself. The minimal fix is `SELECT DISTINCT split.id, token AS key, '' AS key2` in the
outer phone-tier `SELECT` (`:123`). (The `IN` subquery is unaffected — it already `GROUP BY token`.)

Add a real-DB regression test under `backend/services/duplicate_service_real_db_test.go` (use
`database.InitDB` per `/CLAUDE.md` backend trap #1): seed one contact with two phone numbers that share a
`PhoneKey` (and a second, unrelated contact to prove it still pairs correctly), run `FindDuplicatePairs`, and
assert that (a) no returned pair has `A.UID == B.UID`, and (b) the cross-contact pair still appears. Verify
the test fails against the unpatched query before restoring the fix.

Optionally harden `addTier` too: `if a == b { continue }` is a cheap second line of defense for any future
tier that could emit a self-row. Do that regardless — it is a one-line invariant that makes the guarantee
structural rather than dependent on one SQL query staying distinct.

## Traps

- `db.Raw` bypasses GORM's soft-delete scope, which is why the query has explicit `deleted_at IS NULL`
  clauses — preserve them; the fix must not re-include soft-deleted contacts.
- The `DISTINCT` must go on the *outer* select (`split.id, token`), not the `WITH` CTE; the `split` CTE still
  needs its duplicate rows so the `IN (… GROUP BY token)` subquery sees the true per-token multiplicity
  across contacts.
- Do not change `FlattenPhones`/`PhoneKey` — the duplicate-token shape is correct and required for cross-
  format search (T69); the scan must tolerate it, not the writer change to suit the scan.
- The e2e already polls the loser's 404 after a merge (T93's WAL note) — this ticket is a *different* bug
  (a self-pair emitted before any merge), not the post-merge stale-read race.

## Done when

- `FindDuplicatePairs` never returns a pair whose two sides share a `VCardUID`.
- A contact with two phones that share a last-10 `PhoneKey` no longer appears as a duplicate of itself, and
  still pairs correctly with a genuinely different contact that shares a number.
- The frontend "Review duplicates" list no longer offers a same-contact pair whose Merge fails with
  `merge_id must differ from keep_id`.
- `cd backend && go build ./... && go vet ./... && go test ./...` green, with the new regression test
  hand-verified to fail against the unpatched query.

## Landing note (2026-08-15)

Both layers landed in `duplicate_service.go`: the phone tier's outer `SELECT` is now `SELECT DISTINCT
split.id, token AS key` (so a contact can never emit the same token twice into a group), and `addTier`
gained an `if a == b { continue }` guard as defense-in-depth for any future tier. The regression test seeds
one contact with two numbers that reduce to the same `PhoneKey` (`+1 800 555 1234` next to `800-555-1234`,
which makes `FlattenPhones` emit the key twice) plus a genuinely different contact sharing the number, and
asserts (a) no pair has `A.UID == B.UID` and (b) the real cross-contact pair is still found. Hand-verified
per `/CLAUDE.md`: with **both** the `DISTINCT` and the `addTier` guard removed the test fails
(`duplicate scan must never pair a contact with itself`); with either fix in place it passes. The `addTier`
guard alone masks the SQL bug (which is why the first hand-verify attempt looked like a no-op — both fixes
must be reverted together to see the failure). Full `go test ./...` green.

