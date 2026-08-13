# T93 — Nothing can scan the address book for duplicates

| | |
|---|---|
| **Platform** | Backend + Web |
| **Rating** | 4 — the root capability behind three separate beta complaints |
| **Size** | L |
| **Depends on** | Nothing. **Blocks** [T92](136-T92-bulk-merge-from-contacts-list.md) (suggested-pairs half) and [T96](140-T96-import-duplicate-merge-review.md). |
| **Status** | **TO BE DONE** |
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
