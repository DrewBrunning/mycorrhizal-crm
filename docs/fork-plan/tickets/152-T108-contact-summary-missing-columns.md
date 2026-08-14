# T108 — `GET /contacts` returns an empty nickname and circles on every row

| | |
|---|---|
| **Platform** | Backend |
| **Rating** | 3 — two DTO fields that have never carried data |
| **Size** | XS — two strings in a slice, plus deciding what to do about `circles` |
| **Depends on** | Nothing |
| **Status** | **DONE** (2026-08-14) |
| **Source** | Not reported. Found while checking what the list DTO carries, during the 2026-08-13 beta triage. |

## Why this exists

`ContactSummary` declares `Nickname` (`backend/models/contact_summary.go:35`) and `Circles`
(`:43`), and `NewContactSummary` reads `c.Nickname` (`:65`) and `c.Circles` (`:73`) to populate them. The
doc comment at `:26-29` says both were added specifically because ContactsPage renders them per row.

But the list query selects a fixed column set, and **neither column is in it** —
`contactSummaryColumns` (`backend/controllers/contact_controller.go:28-31`):

```go
"id", "vcard_uid", "firstname", "lastname", "fn", "email", "phone", "birthday", "org",
"photo", "photo_thumbnail", "archived"
```

`contactListColumns` (`:36`), `contactNameSortedColumns` (`:40`) and `contactFeedColumns` (`:44`) all
derive from it, and the `Select` at `:181` / `:324-326` applies it. GORM therefore never loads `nickname`
or `circles`, and `NewContactSummary` reads the zero value at every call site (`:192`, `:281`, `:440`). No
`Preload` covers them either — the three at `:387-391` are for notes/activities/reminders.

So `GET /contacts` has always returned `"nickname": ""` and `"circles": null` for every row.

**Nobody noticed for two reasons.** `ContactsPage.tsx:405` renders `contact.nickname` via
`summaryToLegacyContact` (`frontend/src/api/contacts.ts:624-639`, field at `:630`) — an empty nickname just
renders nothing. And the circle chips on the list are fed from a separate `useCircles()` lookup
(`ContactsPage.tsx:48`, `:417`), not from the DTO at all. The client DTO mirror
(`frontend/src/api/contacts.ts:345-359`) has drifted the other way: it declares `nickname` but has no
`circles` field.

## What to build

1. **Add `nickname` to `contactSummaryColumns`** (`contact_controller.go:28-31`). It is a plain column on
   `contacts`, cheap, and the DTO already promises it.
2. **Decide `circles` deliberately.** `Contact.Circles` (`backend/models/contact.go:97`) is the **legacy
   flat JSON column**, superseded by `circle_members` in [T2](05-T2-circle-tag-triage.md)/[T3](06-T3-circle-tag-backend.md).
   Two honest options, and this ticket must pick one rather than leaving both:
   - **Preferred: drop `Circles` from `ContactSummary` entirely** (`contact_summary.go:43`, `:73`) and from
     the doc comment at `:26-29`. Nothing consumes it — the list's chips come from `useCircles()` — and
     shipping the legacy flat column in a current DTO invites new consumers onto data that
     [T3](06-T3-circle-tag-backend.md) deprecated. Remove `circle_legacy`'s DTO sibling only; leave the
     `circle_legacy` *query param* at `contact_controller.go:378-380` alone, it has its own migration story.
   - Alternative: keep the field and populate it properly from `circle_members`, which means a join or a
     second query per page. Only worth it if a real consumer is planned — and if so, that consumer belongs
     in the same change.
3. **Resync the client DTO** (`frontend/src/api/contacts.ts:345-359`) with whatever the Go struct ends up
   being. Add a comment noting it is a hand-synced mirror, per `/CLAUDE.md` frontend trap #4.

## Traps

- **Do not just add both columns and move on.** `circles` would then start returning the legacy flat
  values, which for any contact migrated by T2 are stale relative to `circle_members` — shipping wrong data
  is worse than shipping empty data.
- The four column slices are built with `append(append([]string{}, …))` precisely so appends can't leak
  into the base slice's backing array (comment at `:33-35`). Keep that shape when editing.
- `contactFeedColumns` (`:44`) feeds the `?since=` change feed ([T17](17-T17-change-feeds.md)). Widening
  the base slice widens that payload too — fine for `nickname`, worth a glance for anything larger.
- A test asserting the fix must read the **raw JSON**, not a decoded Go struct: decoding makes "absent" and
  "empty" indistinguishable, which is exactly the hazard `/CLAUDE.md` frontend trap #8 describes.

## Done when

- `GET /contacts` returns a populated `nickname` for a contact that has one.
- `circles` is either gone from the DTO or populated from `circle_members` — not left declared-and-empty.
- The frontend `ContactSummaryDTO` matches the Go struct field for field.
- A raw-JSON test pins the nickname, and would fail against the current column list.
- `cd backend && go build ./... && go vet ./... && gofmt -l . && go test ./...` green.
- `cd frontend && npx tsc --noEmit && npx vitest run` green.
- `backend/openapi.yaml` matches the final DTO shape (the drift test enforces it).

## Landing note (2026-08-14)

Took the preferred option: `nickname` added to `contactSummaryColumns` (and its three derived slices, via
the existing `append(append([]string{}, …))` chain — no shape change needed there), `circles` removed from
`ContactSummary`/`NewContactSummary` entirely, not populated. `frontend/src/api/contacts.ts`'s
`ContactSummaryDTO` already matched this target shape by coincidence (had `nickname`, never had `circles`)
— just added the "hand-synced mirror, must stay in sync" comment per trap #4. `openapi.yaml`'s
`ContactSummary` schema had already grown an honest caveat on `circles` ("the list query does not select
this column... do not rely on it being populated") from some earlier pass that noticed the symptom without
tracing it to the cause — removed now that the field itself is gone.

New raw-JSON regression test (`TestGetContacts_SummaryHasNicknameNoCircles`,
`contact_controller_test.go`) hits the real `GetContacts` handler and asserts against the parsed JSON map,
not a decoded struct — catches both the nickname value and a stray `circles` key the removed Go field
alone wouldn't reveal. Hand-verified by reverting the column-list fix and confirming the test fails.

Found and fixed the actual root cause of "nobody noticed for two reasons" while touching this: the
pre-existing `TestNewContactSummary_IncludesNicknameAndCircles` unit test (now
`TestNewContactSummary_IncludesNickname`, `circles` assertions removed) called `NewContactSummary` directly
against a hand-built `Contact{Nickname: "..."}` — it always passed, because the real bug was in the
controller's `Select()` column list, a layer that test never touched. Left as a mapping-logic unit test
(still legitimate for that narrower purpose), with a comment pointing at the new controller-level test as
the one that actually covers the query.

`go build/vet/gofmt/test` and `tsc --noEmit`/`vitest run` both green.
