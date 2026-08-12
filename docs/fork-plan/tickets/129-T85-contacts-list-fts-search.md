# T85 — `GET /contacts?search=` should use FTS5, so search composes with the list's filters

| | |
|---|---|
| **Platform** | Backend |
| **Rating** | 4 — the enabling half of the Contacts/Search fold |
| **Size** | S–M — one query clause, one exported helper, no migration |
| **Depends on** | Nothing. [T11](24-T11-search-fts5.md)'s `contacts_fts` (migration `000007`, widened by `000010`/`000020`) already exists and is trigger-maintained. |
| **Blocks** | [T86](130-T86-web-fold-search-into-contacts.md) (web fold), [T87](131-T87-android-fold-search-into-contact-list.md) (Android parity) |
| **Status** | **DONE**, 2026-08-12 — see landing note below |
| **Source** | Filed 2026-08-12 out of the design pass for the "combine Contacts and Search" idea (Deferred → Feature ideas, from 2026-08-11 testing notes). |

## Why this exists

There are **two search engines** over contacts, and they can't do each other's job:

| | `applyContactSearch` ([contact_controller.go:97](../../../backend/controllers/contact_controller.go)) | `services.Search` ([search_service.go:145](../../../backend/services/search_service.go)) |
|---|---|---|
| Matching | `LIKE '%term%'` over flat columns | FTS5 prefix-token `MATCH`, bm25-ranked, snippets |
| Covers | contacts only | contacts, notes, activities |
| Filters | `circle`, archived, `sort`, `order`, `includes` | `household_id` only |
| Paging | T17/T73 keyset cursor | `limit` only, capped at 50 per section |
| Response | `ContactSummary` | three heterogeneous hit groups |

T86 folds the Search page into the Contacts page, which means one query must be **both** good at
matching **and** composable with circle/archived/sort/cursor. Today neither is.

Growing `/search` into that endpoint is the wrong direction: two of its three response groups
(notes, activities) have no circle/archived/sort meaning at all, so those params would be no-ops for
two-thirds of the payload, on top of a filtered top-50 with no way to reach row 51. Its
`household_id` param is already the first step down that path — stop there. `GET /contacts` already
owns every dimension the merged page needs, so the work is to make **its** `search=` good, not to
rebuild the list endpoint inside the search controller.

## The load-bearing decision: FTS as a *filter*, not a *ranker*

`services.Search` orders contact hits by `contacts_fts.rank`
([search_service.go:210](../../../backend/services/search_service.go)) — bm25. **bm25 cannot back
the list's cursor.** It is not a stable unique key, so "rows after `(rank, id)`" is not expressible
as a keyset predicate without materializing the whole ranked set; `cursorPredicate` /
`nameCursorPredicate` have nothing to bite on.

Used as a *filter*, it composes with everything already there:

```sql
contacts.id IN (SELECT rowid FROM contacts_fts WHERE contacts_fts MATCH ? AND contacts_fts.user_id = ?)
```

The existing `(updated_at, id)` / `(sort_name, id)` cursor, `sort=`, `order=`, `circle` and the
archived filters keep working **unchanged** — FTS narrows the row set, the existing ORDER BY orders
it.

The cost is no relevance ordering on the contacts list. That is accepted: at personal-CRM scale the
job is finding a name, not ranking a corpus, and under [T77](121-T77-web-contacts-list-sort-control.md)
the user has explicitly chosen a sort anyway. A `sort=relevance` mode (which would have to switch to
offset pagination for that mode only) is **deliberately out of scope** — file it if it is ever
actually wanted, don't design around it now.

## What to build

1. **Export the match-expression logic from `services`.** `ftsQuery`
   ([search_service.go:89](../../../backend/services/search_service.go)) and `phoneFTSMatch`
   ([search_service.go:128](../../../backend/services/search_service.go)) are unexported, and
   `Search` picks between them via `PhoneQueryTokens` (already exported, already called by
   `applyContactSearch`). Add one exported helper that encapsulates the whole choice — e.g.
   `services.ContactFTSMatch(term string) (expr string, ok bool)` returning the phone-shaped
   expression for a phone-shaped term and the plain prefix expression otherwise, `ok=false` for a
   term with no usable tokens. Refactor `Search` to call it so the two paths cannot drift.

2. **Add the FTS clause to `applyContactSearch`, `OR`-ed with the existing LIKE clause.** Additive,
   not a replacement — see the first trap. The subquery carries `contacts_fts.user_id = ?` as well,
   matching the defence-in-depth double-scoping `Search` already does (it filters both
   `contacts_fts.user_id` and `c.user_id`).

3. **Gate the FTS clause at two runes**, matching `Search`'s own gate
   ([search_service.go:153](../../../backend/services/search_service.go)). Below that, LIKE-only —
   which is exactly today's behavior, so single-character search does not change at all.

4. **Nothing else changes.** No migration (`contacts_fts` and its triggers already exist, and
   `RebuildSearchIndex` already covers reindexing). `/search` is untouched and keeps its contacts
   group — it remains the only cross-entity query and the home of `resolved_relation`.

## Traps

- **LIKE and FTS do not match the same rows, and this param has three live consumers.** LIKE is
  substring — `ann` finds `Joanne`. FTS is token-prefix — `"ann"*` does **not** find `Joanne`.
  `applyContactSearch` has exactly one call site
  ([contact_controller.go:338](../../../backend/controllers/contact_controller.go)), but that call
  site serves web's `ContactsPage`, web's AppBar autocomplete
  ([App.tsx:116](../../../frontend/src/App.tsx)) **and** Android's contact list
  (`ApiClient.listContacts`). Replacing the clause would silently change results for all three.
  `OR`-ing is strictly additive: every match that works today still works, and the FTS matches are
  new. Do not "simplify" it into a replacement later without re-reading this paragraph.
- **`contacts_fts` indexes archived and soft-deleted rows.** The subquery must not be trusted to
  filter them — the outer query's existing `archived` and GORM soft-delete conditions do that, and
  they still apply because the FTS clause is only ever a `WHERE … AND (fts OR like)` narrowing.
  Verify with a test that a soft-deleted contact matching the term is not returned.
- **Cross-user scoping is the highest-risk part.** The outer query is already
  `Where("user_id = ?", userID)`, so the subquery cannot leak by itself — but add
  `contacts_fts.user_id = ?` anyway and pin it with a test, per T11's own "test it explicitly."
- **`applyContactSearch` builds one `clause` string with a positional `args` slice.** The phone
  branch already appends conditionally. Adding an OR-ed subquery means the new placeholders must be
  appended in the right order — an off-by-one here silently searches for the wrong string rather
  than erroring.

## Done when

- `cd backend && go build ./... && go vet ./... && gofmt -l . && go test ./...` green.
- Tests prove, against `database.InitDB` (not `AutoMigrate` — the FTS triggers only exist in the
  real migrated schema): a contact findable only by FTS prefix-token is returned by
  `GET /contacts?search=`; a contact findable only by LIKE substring is **still** returned;
  `search=` composes with `circle=`, `include_archived` and `sort=name` in one request; the cursor
  pages correctly through a searched result set with no duplicate or skipped rows; a cross-user
  contact is never returned; a soft-deleted contact is never returned; a one-character term behaves
  exactly as it does today.
- Hand-verified per `/CLAUDE.md`: remove the `contacts_fts.user_id` condition, confirm the
  cross-user test fails, restore. Break the OR into a replacement, confirm the LIKE-substring test
  fails, restore.

### Post-alpha note

Real production data exists. This ticket adds **no migration** and changes no schema — `contacts_fts`
is derived data that already exists and is rebuildable via `services.RebuildSearchIndex`. The only
behavior change is that `GET /contacts?search=` returns a **superset** of what it returns today.

## Landing note

**Shipped 2026-08-12.**

**`services.ContactFTSMatch(term string) (expr string, ok bool)`** (`services/search_service.go`)
centralizes the phone-vs-plain match-expression choice that used to live only inside `Search`: the
phone-shaped expression (`phoneFTSMatch`) for a phone-shaped term (`PhoneQueryTokens`), the plain
prefix expression (`ftsQuery`) otherwise, `ok=false` when the term tokenizes to nothing. `Search` was
refactored to call it so the two paths cannot drift, per the ticket's item 1.

**`applyContactSearch`** (`controllers/contact_controller.go`) gained a new signature
(`userID uint` added) and, at the end of its existing clause-building, an OR-ed subquery:
`contacts.id IN (SELECT rowid FROM contacts_fts WHERE contacts_fts MATCH ? AND contacts_fts.user_id
= ?)`, gated behind `len([]rune(strings.TrimSpace(searchTerm))) >= 2` — the same gate `Search` uses.
The LIKE clause is untouched and still runs first; FTS only adds matches on top, never replaces any.

**Test-infra fix, not scope creep**: `applyContactSearch` now references `contacts_fts` on *every*
two-plus-rune search, but that table is hand-written migration SQL invisible to `AutoMigrate`. Every
other controller test in the package shares one `setupRouter()` helper (`activity_controller_test.go`)
built on `AutoMigrate`, and a dozen of them call `GET /contacts?search=` — all started 500ing
(`no such table: contacts_fts`) the moment the clause went in. Fixed by creating an empty, trigger-
less `contacts_fts` inside that shared helper: the FTS clause then contributes nothing (empty table),
so those tests keep exercising exactly the LIKE behavior they always did, with no change to their
assertions. FTS-specific matching is covered separately, against the real migrated schema, in the new
`controllers/contact_fts_search_test.go`.

**Tests** (`controllers/contact_fts_search_test.go`, `database.InitDB` real schema): an
FTS-only match (`org` is indexed into `contacts_fts` but was never part of the LIKE clause, so a hit
on organization text could only ever come from T85's new subquery); a LIKE-only match still works
(`"xand"` inside `"Alexander"` — a mid-token substring LIKE finds but FTS token-prefix never would,
pinning the "OR, never replace" trap); one request combining `search=`, `circle=`, `include_archived`
and `sort=name` returns exactly the intersection in name order; a search-narrowed cursor walk with
`limit=2` returns every matching row exactly once; a cross-user match and a soft-deleted match are
each never returned; a one-character term is proven identical to pre-T85 behavior by showing the
*same* org-only contact is invisible at one rune and found at two.

Two further tests were added during this ticket's own review pass, both covering things the
done-when list did not name:

- **`TestGetContactsSearch_SpecialCharactersDoNotError`** — the contacts-list counterpart to
  `services.TestSearch_SpecialCharactersDoNotError`. T85 put user input on a path to FTS5 `MATCH`
  that it had never reached from this endpoint before, and an unescaped quote or bare operator is an
  FTS5 syntax error — i.e. a **500 on the main contact list**, which has three live consumers where
  `/search` has one. `ftsQuery`'s quoting already prevented it (verified empirically before writing
  the test), but nothing pinned that the protection reached *this* caller rather than being inherited
  by assumption. Inputs are `url.QueryEscape`d so `%`, `+` and `&` actually arrive intact.
- **`TestGetContactsSearch_NameSortedCursorPagesSearchedSet`** — the original paging test walked the
  default `(updated_at, id)` cursor. The `(sort_name, id)` cursor is the one T86 will actually ship
  on, since [T77](121-T77-web-contacts-list-sort-control.md) makes name the web default, and it is
  the pairing the "filter, not ranker" decision is really about. Pins that the FTS clause narrows the
  set without disturbing the name cursor's total order.

**OpenAPI**: the `search` parameter's description on `GET /contacts` **was** updated — the earlier
claim of "no OpenAPI change" was wrong. It read "across name/nickname/email/phone (including
secondary array entries)", which had already gone stale at [T38](47-T38-search-address-fields.md)
(addresses missing) and which T85 widened further (organization, and prefix-token matching alongside
substring). It now describes the union semantics, the two-character FTS threshold, composability with
the other filters, and the deliberate absence of a relevance sort. Spec still validates; the frontend
API client is hand-written, so there is no codegen to regenerate.

**Hand-verified per `/CLAUDE.md`, both reverted after confirming**: dropping `contacts_fts.user_id =
?` from the subquery did **not** fail the cross-user test — expected and consistent with the ticket's
own note that the subquery can't leak by itself (the outer `Where("user_id = ?", userID)` already
scopes every row the FTS `IN` could ever select; the subquery condition is defense-in-depth, kept
anyway). Turning the `OR` into a replacement (bare FTS clause, no LIKE) **did** fail
`TestGetContactsSearch_LIKEOnlyMatchStillWorks` and three pre-existing LIKE-path tests
(`TestGetContactsSearchMultiValue`, `TestGetContactsSearchMatchesAddresses`,
`TestGetContactsSearchMatchesPhonesNormalized`) — confirming the additive-OR requirement is load-
bearing, not decorative.

The two review-pass tests were hand-verified the same way: removing `ftsQuery`'s token quoting made
the special-characters test fail on `it's` with a 500 `DATABASE_ERROR`; dropping the
`applyContactSearch` call entirely made the name-sorted paging test fail by leaking the
deliberately-planted non-matching contact (`Nadir`) into the middle of the walk. Both restored.

`go build ./... && go vet ./... && gofmt -l . && go test ./...` green. No migration. One doc-only
OpenAPI edit (the `search` parameter description — see above). Unblocks
[T86](130-T86-web-fold-search-into-contacts.md) and
[T87](131-T87-android-fold-search-into-contact-list.md).
