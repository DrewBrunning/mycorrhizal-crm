# T93 — Nothing can scan the address book for duplicates

| | |
|---|---|
| **Platform** | Backend + Web |
| **Rating** | 4 — the root capability behind three separate beta complaints |
| **Size** | L |
| **Depends on** | Nothing. **Blocks** [T92](136-T92-bulk-merge-from-contacts-list.md) (suggested-pairs half) and [T96](140-T96-import-duplicate-merge-review.md). |
| **Status** | **DONE** (2026-08-14) |
| **Source** | Beta testing note, 2026-08-13: *"We should add a means of bulk duplication checking — checking for duplicate contacts and suggesting merge, checking for duplicate phone numbers."* |

## Why this exists

There is exactly one duplicate detector in the codebase and it can only answer one question:
*"does this one incoming row match anything?"*

`services.DetectDuplicate` (`backend/services/import_service.go:728-786`) takes a candidate's email, name
and phone and returns a single `*models.DuplicateMatch` (`backend/models/import.go:45-57`). Three tiers,
first match wins:

1. **email** (`:731-743`) — `LOWER(email) = LOWER(?)` on the flat `email` column only; the `emails` JSON
   array is not scanned.
2. **name** (`:745-759`) — exact `LOWER(firstname)` + `LOWER(lastname)`, only when both are non-empty. No
   nickname, no fuzz.
3. **phone** (`:761-784`, added by [T68](112-T68-phone-dedup-country-code-normalization.md)) — loads
   **every** contact with a non-empty flat `phone` into memory and compares `models.PhoneKey`
   (`backend/models/phonekey.go:15-24`, digits only, last 10 kept, `""` below 7 digits). Flat column only.

Its only three call sites are import preview builders — `import_service.go:266` (CSV), `:360` (VCF), `:549`
(JSContact). Nothing else calls it, there is no `/duplicates` route in `backend/routes/routes.go`, and
there is no `FindDuplicates`/`ScanDuplicates` symbol anywhere in `backend/`, `frontend/src` or `android/`.
[N1](01-N1-contact-merge.md) §6 listed a find-duplicates view as an option; it was never built.

Android has its own weaker copy — `ImportContactsViewModel.findDuplicate`
(`android/feature/import/.../ImportContactsViewModel.kt:63-71`): email-then-phone against the local Room
cache only, no name tier, no `PhoneKey` normalization.

## What to build

### 1. `GET /contacts/duplicates`

Returns candidate **pairs**, not per-row matches. Response shape:

```json
{
  "pairs": [
    {
      "a": { "id": 12, "uid": "…", "fn": "Jane Doe", "primary_email": "…", "primary_phone": "…" },
      "b": { "id": 87, "uid": "…", "fn": "Jane Doe", "primary_email": "…", "primary_phone": "…" },
      "reasons": ["email", "phone"],
      "confidence": 0.95
    }
  ],
  "total": 14
}
```

- Each side is a `ContactSummary` (`backend/models/contact_summary.go:30-49`) — same DTO the list endpoint
  returns, so the web client can render rows with existing components.
- `reasons` is the full set of tiers that matched, not the first one. A pair matching on both email and
  phone is a much stronger candidate than one matching on name alone, and the UI needs to say so.
- Ownership-scoped by `user_id` like every other handler (`/CLAUDE.md` backend trap #5). Archived contacts
  are included but flagged; soft-deleted ones are excluded.
- Cursor-paginated via the existing `GetPaginationParams` idiom.

### 2. Set-wide detection, not n² `DetectDuplicate` calls

`DetectDuplicate`'s phone tier is already an O(n) in-memory scan **per candidate**. Calling it once per
contact makes the whole scan O(n²) with a full table load inside the loop. Do not do that.

Instead, compute each tier as a SQL grouping over columns that already exist and are already maintained by
`Contact.BeforeSave`:

- **email** — `GROUP BY LOWER(email) HAVING COUNT(*) > 1`, over non-empty values.
- **name** — `GROUP BY LOWER(firstname), LOWER(lastname) HAVING COUNT(*) > 1`, both non-empty. `sort_name`
  ([T73](117-T73-contacts-list-sort-control.md), already lowercased and indexed) is the cheaper key if it
  is a faithful firstname+lastname join — **check that before using it**, it is `COALESCE`-guarded and may
  fall back to other fields.
- **phone** — group on `models.PhoneKey`. There is no stored column for the key itself; `phones_normalized`
  ([T69](113-T69-phone-search-tokenization.md)) holds every phone's digits *and* its last-10 key,
  space-joined. Either tokenize that in SQL or add a dedicated indexed key column. Prefer reading
  `phones_normalized` — a new column means a migration on real production data for a feature that can be
  computed without one.

Then union the tier results into pairs, deduping `(a,b)` against `(b,a)`.

**Phone dedup covers the report's second clause explicitly** ("checking for duplicate phone numbers"):
because `phones_normalized` covers *all* of a contact's numbers rather than just the flat primary, this
tier finds shared numbers `DetectDuplicate` misses today.

### 3. Web review surface

A "Review duplicates" page or dialog reachable from the Contacts page, listing the pairs newest-strongest
first, each with a **Merge** button that opens `MergeContactsDialog` on that pair (see
[T92](136-T92-bulk-merge-from-contacts-list.md) step 5) and a **Not a duplicate** dismissal.

**Dismissals must persist.** Otherwise the list re-suggests the same twins/father-and-son pair on every
visit, and the feature becomes noise. Store them in a small table keyed on the ordered uid pair
(`(user_id, uid_low, uid_high)` unique). Per `/CLAUDE.md` trap #7 that is an edge-shaped join row with a
natural key, so it **hard-deletes** and needs a migration adding it to `DeleteContact`'s and `DeleteUser`'s
cascade lists (`backend/controllers/contact_controller.go`'s `deleteContactAssociations` is the canonical
checklist).

## Traps

- **`DetectDuplicate` stays as-is for the import path.** Import asks a genuinely different question
  (one incoming row vs. the table). Refactor to share the *key* functions (`PhoneKey`, the email/name
  normalizers) — do not try to make one function serve both shapes.
- **Name matching is the false-positive tier.** Exact firstname+lastname will pair a father and son, and
  will pair two real people who share a common name. That is why `reasons` and `confidence` exist and why
  dismissal must persist. Do not add fuzzy/edit-distance matching in this ticket — it makes the false
  positives worse, not better, and needs its own design pass.
- Pets and relationship-only contacts ([T103](147-T103-contacts-list-has-contact-info-filter.md)) will
  cluster hard on the name tier (several "Mum" entries, several unnamed pets). Consider excluding contacts
  with no contact info from the name tier specifically.
- `/CLAUDE.md` backend trap #1: test against `database.InitDB`, not `AutoMigrate`.

## Done when

- `GET /contacts/duplicates` returns pairs for a database seeded with a known email dupe, a known name
  dupe, and a known phone dupe that differs by country code and punctuation (`+18005551234` vs
  `(800) 555-1234` — the [T68](112-T68-phone-dedup-country-code-normalization.md) case).
- A contact with a duplicate on a *non-primary* phone number is found — the thing `DetectDuplicate` cannot
  do today.
- The scan issues a bounded number of queries regardless of contact count; pinned by a test asserting the
  query count, not just the result.
- Dismissing a pair keeps it dismissed across reloads, and deleting either contact cleans up the dismissal
  row.
- Another user's contacts never appear in the results.
- New strings translated in all five locales.
- `cd backend && go build ./... && go vet ./... && gofmt -l . && go test ./...` green.
- `cd frontend && npx tsc --noEmit && npx vitest run` green, plus a Playwright spec.
- `backend/openapi.yaml` covers the new route.

## Landing note

**Delivered 2026-08-14.** All three parts shipped together (backend engine, dismiss storage, web
review surface), because the review surface is the point of the engine and the Merge button needs the
pair-mode dialog — which T92 step 5 defers to "once T93 lands." That deferral left a chicken-and-egg:
the review dialog can't open `MergeContactsDialog` on a pair until the dialog can express a pair. The
minimal lift (`MergeContactsDialog`'s `pair` mode — the props are now a discriminated union, so a call
site cannot mix single- and pair-mode props: explicit keeper radio + swap, no search picker) is done
here and directly serves T92 step 5; T92's remaining work (the bulk-actions-bar entry point,
refresh/selection semantics) is unaffected.

Decisions worth recording:

- **Offset pagination, not cursor.** The ticket's "cursor-paginated via the existing `GetPaginationParams`
  idiom" names the offset helper (`GetPaginationParams` *is* offset), and pairs are computed fresh per
  request — there is no stable row identity to cursor on. The response carries `total` + `page` + `limit`.
- **Name tier excludes no-contact-info contacts** (the ticket's "consider" trap, made). Two contacts
  both named "Mum Smith" with no email/phone are relationship stubs/pets, not a merge candidate.
- **Phone tier tokenizes `phones_normalized` in SQL** (no new column — the ticket's stated preference).
  A contact's full-digit tokens and last-10 `PhoneKey` tokens are space-joined in that column; a
  `json_each` split over a space→`","` rewrite yields one row per token, filtered to ≥7 digits. This
  covers every number a contact has, not just the flat primary — the report's "duplicate phone numbers"
  clause, and the thing `DetectDuplicate` cannot do.
- **`sort_name` not used for the name tier.** It's `COALESCE`-guarded and falls back to firstname alone,
  so it is not a faithful firstname+lastname join (the ticket's "check before using" trap).
- **Confidence is a pure function of the reason set**, not a distance heuristic: all three → 0.98,
  email+phone → 0.95, name+email or name+phone → 0.9, phone → 0.75, email → 0.7, name → 0.5. Sorting
  is confidence desc then `(a.id, b.id)`, which makes offset pagination deterministic.
- **Dismissal is idempotent (200, not 409)** — a double-click is not an error. This deliberately differs
  from the household-suggestion dismissal's 409, which is a different shape (a group, not a pair). The
  idempotency guard is the unique index itself: the controller INSERTs and treats a `UNIQUE constraint
  failed` error as "already dismissed" (the house idiom from `admin_user_controller.go`) — a
  count-then-create would race and 500 under two concurrent dismissals of the same pair.
- **Query count is exactly 5** — one per tier + summaries + dismissals — pinned by a counting-GORM-logger
  test over 150 contacts. This is the ticket's bounded-query guarantee made concrete.

## Review pass, 2026-08-14 (post-landing self-review)

All findings fixed in one follow-up:

- **Name-tier grouping key is a TUPLE, never a separator-joined string.** The first version concatenated
  `LOWER(firstname) || '|' || LOWER(lastname)`, so `"A|B"+"C"` and `"A"+"B|C"` collapsed onto `a|b|c` and
  produced a false pair. The SQL now partitions and selects the two name columns separately and Go groups
  by `[2]string{first, last}`; a collision test seeds both shapes plus a genuine `(A|B, C)` duplicate and
  asserts only the true pair forms.
- **`duplicateSummaryColumns` moved into `models.ContactSummaryColumns`** — the service and the
  controllers' `GetContacts` now read one list instead of two hand-synced mirrors (the drift hazard
  `/CLAUDE.md` trap #4 warns about).
- **Dismissal idempotency made race-safe** (unique-index tolerance, above) with an 8-way concurrent
  dismiss test asserting all 200s and exactly one row.
- **`MergeContactsDialog` props became a discriminated union**, eliminating the silently-broken-caller
  hazard of optional single-mode props; pair mode fires exactly one preview (the keeperUid reset no
  longer doubles it), and the keeper RadioGroup gained an accessible name.
- The review dialog's multi-page fetch now fills the list incrementally instead of holding a blank
  spinner until every page lands; the "Review duplicates" button icon is `DifferenceIcon`, not a copy
  icon. Phone-pair confidence (`0.75`) is asserted; the test's `window.confirm` spy is restored.

The one genuinely surprising find came from the e2e run, not the code: **a merge POST returning 200 is
not immediately visible to a follow-up GET.** The loser GET intermittently returned 200 right after a
successful commit; polling it converged on 404 within a few hundred ms. Cause is SQLite WAL read
snapshots on the backend's pooled connections (a connection mid-read-transaction serves the pre-commit
state), not anything in the merge path — the direct-API merge loop was 15/15 clean. The e2e asserts
the loser's 404 via `expect.poll` rather than a single request. No backend change made: a user's UI
never issues an immediate post-merge GET of the loser, and the review dialog's refetch is a scan, not a
row read.
