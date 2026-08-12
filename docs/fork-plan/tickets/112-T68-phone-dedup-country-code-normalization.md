# T68 — Phone comparison doesn't reconcile country code, so real duplicates go undetected

| | |
|---|---|---|
| **Platform** | Backend |
| **Rating** | 4 — undermines the existing N1 dedup feature; gets worse with every import |
| **Size** | S — one function, two call sites |
| **Depends on** | Nothing. [T69](113-T69-phone-search-tokenization.md) consumes the function this ticket defines, so land this one first. |
| **Status** | **DONE** (2026-08-12) |
| **Source** | Testing notes, 2026-08-11: "Phone numbers don't have any standard formatting so +18005551234, (800)555-1234, 800-555-1234, etc all register as different numbers" |

### Landing note (2026-08-12)

Implemented `PhoneKey` — a last-10-significant-digits canonical comparison key with a 7-digit minimum. Both call sites (`DetectDuplicate` and `unionPhones`) now use it. `normalizePhoneForComparison` is kept for T69's full-digit FTS5 indexing.

Tests: 13 new/updated tests across 3 files — 11 PhoneKey unit tests (country code, UK trunk prefix, exactly 10, punctuation, >10 keeps last 10, 7-digit floor, 6-digit returns empty, empty string, no digits, whitespace, very long, non-Latin digits), 4 merge-service tests (extended existing, added country-code, three-way, UK prefix, 3 empty-key guard tests), and 5 real-DB DetectDuplicate tests (country code, UK prefix, too-short no match, two-short no match, punctuation). All hand-verified to fail against the pre-fix `normalizePhoneForComparison`.

## Why this exists

There's already one shared phone-comparison helper, and it already documents its own gap —
`backend/services/contact_merge_service_test.go:34-46` states outright: *"`normalizePhoneForComparison`
strips non-digit punctuation but does not reconcile an explicit country code against its absence."*

```go
// backend/services/import_service.go:727-736
func normalizePhoneForComparison(phone string) string {
    var normalized strings.Builder
    for _, r := range phone {
        if r >= '0' && r <= '9' {
            normalized.WriteRune(r)
        }
    }
    return normalized.String()
}
```

It's used in exactly two places: `DetectDuplicate` (`import_service.go:739-795`, import-time only —
there is no standing "find duplicates among existing contacts" feature per
[N1](01-N1-contact-merge.md)) and `unionPhones` (`backend/services/contact_merge_service.go:134-156`,
used when a user manually merges two contacts).

Concretely: `"(800)555-1234"` and `"800-555-1234"` both normalize to `"8005551234"` and **do**
dedupe today. But `"+18005551234"` normalizes to `"18005551234"` — a different string, an 11-digit
mismatch against the 10-digit forms — so it is **not** recognized as the same number. This is
exactly the bug report's example.

**Real functional impact, not cosmetic:**
- **Import duplicate detection** — if an existing contact has `+18005551234` and an imported
  CSV/vCard brings in `(800) 555-1234` for the same person, the phone match misses it and a
  duplicate contact is silently created instead of being flagged, unless name or email also happens
  to match.
- **Manual merge dedup** (`unionPhones`) — lower severity, only fires after a user has already
  picked a pair to merge; a mismatch means the merged contact keeps two `Phones[]` entries for one
  real number (clutter, not silent loss).

**Not in scope here**: phone search (FTS5 tokenizes on the raw column, so differently-punctuated
queries also miss matches) is a related but structurally separate bug — filed as
[T69](113-T69-phone-search-tokenization.md).

**Not in scope here either**: rewriting what's actually *stored* to a canonical E.164 form. That
would touch `Contact.BeforeSave`, `ApplyRecordToContact`/`RecordForContact`, all three export
adapters, CardDAV's `vcard_mapper.go`, and the frontend phone field, and — per `/CLAUDE.md`'s
data-safety rules — would need a deliberate backfill decision for already-stored data rather than a
silent rewrite. This ticket is scoped to the comparison-only fix, which is smaller and lower-risk.

## The approach — decided 2026-08-11, not left to implementation time

The original draft left "vendor a phone-parsing library vs. write a heuristic" open. **Decided: a
heuristic, specifically a canonical *significant-digits key*.** No library.

```go
// PhoneKey reduces a phone number to a comparison key: its digits, keeping at
// most the last 10. Two numbers with the same key are treated as the same
// number. Returns "" for anything with fewer than <min> digits.
func PhoneKey(phone string) string
```

Why last-10-digits rather than the alternatives:

- **It handles the reported case and more.** `+18005551234` → `8005551234`; `(800) 555-1234` →
  `8005551234`; `800-555-1234` → `8005551234`. All three collapse. It also collapses the UK trunk-
  prefix case for free — `+44 20 7946 0958` → `2079460958` and `020 7946 0958` → `2079460958` —
  which a "strip a leading `1`" heuristic would not.
- **It is a pure function to a canonical key, so it is transitive.** The obvious alternative,
  "match if one digit string is a suffix of the other," is *not*: `5551234` would match both
  `8005551234` and `9005551234`, which don't match each other. Today's two call sites only compare
  pairwise so that wouldn't bite yet, but a key is what [T69](113-T69-phone-search-tokenization.md)
  needs to index, and an index cannot be built from a non-transitive relation.
- **It needs no default region.** A libphonenumber-style parse must assume a region to resolve
  numbers written without `+`, which is a product-visible guess that will be wrong for anyone with
  international contacts, *and* still needs a fallback path for numbers it fails to parse.
  `nyaruka/phonenumbers` also carries several MB of generated metadata into a self-hosted binary.
- **Its false-positive shape is narrow and acceptable**: two different numbers sharing the same
  final 10 digits, essentially the same subscriber number in two countries. Compare that against
  today's false *negatives*, which fire on the single most common formatting difference there is.

Set the minimum at **7 significant digits** — below that (`5551234` and shorter) return `""` and
never match, so short/extension-like values can't collapse onto each other. Note this is stricter
than today's `len >= 5` gate in `DetectDuplicate`; keep the stricter one.

## What to build

1. Add `PhoneKey` as described, in `import_service.go` beside the function it replaces. Keep
   `normalizePhoneForComparison` (digits-only, no truncation) if anything else still wants raw
   digits — [T69](113-T69-phone-search-tokenization.md) indexes **both** the full digit string and
   the key, so don't delete it.
2. Point `DetectDuplicate` (`import_service.go:739-795`) and `unionPhones`
   (`contact_merge_service.go:134-156`) at `PhoneKey`. Both get the fix from the one change.
3. Guard the empty key: `PhoneKey("") == ""` and two unmatchable numbers must not compare equal
   through a shared `""`. This is the one way this change could *create* a bug — a naive
   `key(a) == key(b)` would make every too-short number a duplicate of every other.
4. Extend `contact_merge_service_test.go`'s existing gap-documenting test (lines 34-46) to assert the
   fixed behavior instead of the gap, and add an `import_service_test.go` case for the same
   country-code scenario at import time. Cover the empty-key case from item 3 explicitly.
   Hand-verify: confirm the new/updated tests fail against the old function first, per `/CLAUDE.md`.

## Done when

- `+18005551234`, `(800) 555-1234`, and `800-555-1234` are all recognized as the same number by both
  `DetectDuplicate` (import) and `unionPhones` (manual merge).
- Two numbers that are both too short to key do **not** compare equal to each other.
- Updated/new tests in `contact_merge_service_test.go` and `import_service_test.go` cover both, and
  were hand-verified to fail against the pre-fix code.
- `go build ./... && go vet ./... && gofmt -l . && go test ./...` green.
