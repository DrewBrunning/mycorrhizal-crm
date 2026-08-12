# T69 — Phone search misses results because nothing normalizes phone numbers

| | |
|---|---|
| **Platform** | Backend |
| **Rating** | 3 — real search miss, but has a workaround (type it as stored) |
| **Size** | M — migration + triggers + two query paths |
| **Depends on** | [T68](112-T68-phone-dedup-country-code-normalization.md) — this ticket indexes the `PhoneKey` function T68 defines. Land T68 first. |
| **Status** | TO BE DONE |
| **Source** | Found investigating testing notes, 2026-08-11 ("phone numbers... all register as different numbers") — this is the search-side half of that report; T68 is the dedup-side half. |

## Why this exists

Searching for a contact by phone number frequently fails unless the query is typed with the exact
punctuation and grouping the number was stored with — which the user has no way to know or guess.

**There are three separate phone-search code paths, not one.** The original draft of this ticket
only named the first. Confirmed by grep, 2026-08-11:

**Path 1 — FTS5 global search** (`/search`). `contacts_fts`
(`backend/database/migrations/000007_search_fts5.up.sql`, rebuilt by `000010`) indexes the raw
`phone` column with the default `unicode61` tokenizer, which splits on punctuation:
`"(800) 555-1234"` becomes three tokens — `800`, `555`, `1234`. `ftsQuery`
(`backend/services/search_service.go:80-90`) splits the *query* on whitespace only and prefix-matches
each token, so `8005551234` is one long token that prefix-matches none of the three indexed ones.

**Path 2 — the contacts-list search bar** (`GET /contacts?search=`). This is a completely different
mechanism: `applyContactSearch` (`backend/controllers/contact_controller.go:93-108`, called at
`:275`) does `phone LIKE '%<term>%'` against the raw column plus a `json_each` scan of the `phones`
array. A substring match has the identical problem — `%8005551234%` does not occur inside
`(800) 555-1234`. **This is the path the reported search most likely went through**, since it backs
the search box on the Contacts page rather than the global search page.

**Path 3 — Android's offline search**, filed separately as
[T76](120-T76-android-local-fts-phone-search.md). Same bug, different repo module (`cached_contacts_fts`
is Room FTS4 over `primaryPhone`), and it cannot be fixed from the backend.

**A fourth, related gap worth fixing here while the schema is open**: `contacts_fts` indexes only the
flat `phone` column — the contact's *primary* number. The `phones` JSON array is not indexed at all,
so global search cannot find a contact by their second or third number even when typed perfectly.
Path 2 does check `phones` via `json_each`, so the two paths already disagree about what is
findable.

## The approach — decided 2026-08-11

The original draft left two options open. **Decided: option 1, a normalized shadow column**, because
[T38](47-T38-search-address-fields.md) already did exactly this for addresses and the machinery is
proven. Migration `000010_search_addresses.up.sql` is a directly copyable template: it added a
denormalized `addresses_flat TEXT NOT NULL DEFAULT ''` column, populated it from `Contact.BeforeSave`
(`models/contact.go:242`), backfilled existing rows in SQL, and — since FTS5 cannot
`ALTER TABLE ADD COLUMN` — dropped and recreated `contacts_fts` and its three triggers with the new
column. Read that migration before writing this one.

Option 2 (a phone-shaped query path bypassing FTS) is rejected: it would need implementing twice,
once per backend path, and it leaves the index still unable to answer the question.

### Shape of the shadow column

`phones_normalized TEXT NOT NULL DEFAULT ''`, maintained in `BeforeSave` alongside `AddressesFlat`,
containing for **every** entry in `Phones[]` (not just the flat primary, fixing the fourth gap
above) two space-separated tokens:

- the full digit string (`normalizePhoneForComparison`), and
- `PhoneKey` from [T68](112-T68-phone-dedup-country-code-normalization.md) — the last-10-digits key —
  when it differs from the full string.

Emitting both is what makes the match work in both directions: a query of `18005551234` hits the
full-digit token, a query of `8005551234` hits the key token, for a number stored either way.

Query side: in `search_service.go`, detect a phone-shaped term (mostly digits, `+`, and phone
punctuation), and when it is one, match against the normalized digits **and** the key rather than
passing the raw term through `ftsQuery`'s whitespace tokenizer. Do the same normalization in
`applyContactSearch` for path 2 — there it is a `LIKE` against `phones_normalized` instead of
against `phone`/`json_each(phones)`.

## What to build

1. `Contact.PhonesNormalized` field + `phones_normalized` column, populated in `BeforeSave` from all
   of `c.Phones` using `normalizePhoneForComparison` and `PhoneKey`.
2. Migration `000020`: add the column, backfill from the existing `phones` JSON in SQL, drop and
   recreate `contacts_fts` + its three triggers to include it. Follow `000010`'s structure and its
   `json_valid` / `COALESCE` guards so a malformed payload can't violate `NOT NULL`. Write the
   matching `.down.sql`.
3. Update `RebuildSearchIndex` (`services/search_service.go:245-246`) — it has its own hardcoded
   column list for both the `INSERT` and the `SELECT`, and will silently drift if missed.
4. Path 1: phone-shaped query detection + normalized matching in `search_service.go`.
5. Path 2: point `applyContactSearch`'s phone matching at `phones_normalized`.
6. Tests, hand-verified to fail first per `/CLAUDE.md`, against a `database.InitDB` schema (trap #1),
   covering at minimum: query `8005551234` finds a contact stored as `(800) 555-1234`; query
   `(800) 555-1234` finds one stored as `+18005551234`; a **non-primary** phone is findable through
   path 1; and both paths agree on the same fixture.

## Traps

- **This is a real schema migration on real production data.** Unlike the FTS tables themselves —
  which are derived and rebuildable, the argument `000010` leans on — `phones_normalized` is a new
  column on `contacts`. It is additive with a default and backfilled, so no existing data is at
  risk, but the `.down.sql` needs to be right.
- **Don't index only the flat `phone` column** out of habit; that reproduces the fourth gap above.
- **Keep the two backend paths consistent.** They already disagree about non-primary phones; ending
  this ticket with them disagreeing about *formatting* instead would be no better.

## Done when

- Searching by a phone number finds the contact regardless of punctuation or grouping differences
  between query and stored value, through **both** the global search page and the Contacts-page
  search box.
- A contact's non-primary phone number is findable through global search.
- New tests cover the cross-punctuation and non-primary cases on both paths, hand-verified to fail
  against the pre-fix code.
- `cd backend && go build ./... && go vet ./... && gofmt -l . && go test ./...` green.
