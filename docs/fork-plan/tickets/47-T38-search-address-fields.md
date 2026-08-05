# T38 — Search doesn't index address fields

| | |
|---|---|
| **Rating** | 4 — a real gap in an already-shipped R5 feature |
| **Size** | S–M |
| **Depends on** | [T11](24-T11-search-fts5.md) (done) |
| **Alpha** | n/a — real data exists. `contacts_fts` is derived/rebuildable, per T11's own note; safe to change post-alpha |
| **Source** | v0.2.0-alpha real-world testing |

## Why this exists

T11 shipped FTS5 search over contacts/notes/interactions, but **address fields were never
included**. Confirmed against the actual migration (`000007_search_fts5.up.sql`): `contacts_fts`
indexes `firstname, lastname, nickname, email, phone, org` only. Searching "Clark St" finds
nothing, even for a contact whose home address is on Clark St — a genuine, reasonable expectation
of a "search" feature that the shipped version doesn't meet. The legacy non-FTS fallback
(`applyContactSearch` in `contact_controller.go`) has the same gap — it matches
firstname/lastname/nickname/email/phone via `LIKE`, never address.

## What to build

Add address text to the searchable surface. `Contact` addresses are stored as structured JSON
(street/city/region/postal/country per component, per T29's address work), not a flat column, so
this needs either:

- A derived/denormalized flat text column (e.g. `addresses_flat`) maintained alongside the JSON,
  populated by the same triggers/save hooks that already exist for other flattened fields, then
  indexed into `contacts_fts` the same way `org`/`email`/`phone` already are — the
  lowest-friction option, consistent with how the rest of `contacts_fts` already works (flat
  columns, not JSON `json_each` matching); **or**
- Extend the FTS5 triggers to flatten the JSON address array into indexable text directly at
  trigger time (`json_each` over the addresses column, concatenated) — avoids a new denormalized
  column but makes the trigger SQL considerably more complex, and this project's existing pattern
  (`applyContactSearch`'s own `json_each` usage for emails/phones) suggests JSON-aware `LIKE`
  matching is already an accepted technique here, so this may be preferable for consistency.

Either way: rebuild the search index (`RebuildSearchIndex` / `cmd/backfill-search-index`, already
built as re-runnable per T11) after landing, so existing contacts' addresses become searchable
without waiting for their next edit.

## Traps

- **Soft-delete correctness (T11's own trap, re-apply here):** whichever mechanism is chosen must
  respect `deleted_at IS NULL` the same way the existing triggers do, or a soft-deleted contact's
  address becomes findable again through this new path.
- **Sensitivity (`91.13`):** confirm addresses aren't gated behind any sensitivity level that
  would make indexing them into a globally-searchable table a leak — check how `org`/`email`
  (already indexed) handle this today and match it exactly, don't invent a new policy here.
- **User scoping** — same `user_id UNINDEXED` pattern as every other FTS table; this is T11's own
  "highest-risk correctness issue," don't relax it for the new field.
- If choosing the denormalized-column route, remember `CLAUDE.md`'s backend trap 2 — set it via
  whatever hook already derives flat columns from the nested `Card`/`CRM` model
  (`BeforeSave`/`ApplyRecordToContact`), not a direct field mutation that bypasses it.

## Done when

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` green.
- A test proves: searching a street name finds the contact whose address contains it; a
  soft-deleted contact's address is not findable; a cross-user search does not leak another
  user's address matches.
- Hand-verified: `RebuildSearchIndex` run against existing seeded contacts makes their addresses
  searchable without editing them first.
