# M13 — Real full-text search on Android

| | |
|---|---|
| **Rating** | 4 |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | Nothing — T11's FTS5 `/search` endpoint already exists and serves web today |
| **Status** | TO BE DONE |

`MycorrhizalApp.kt:453` wraps the `"search"` drawer route in `PlaceholderScreen`. The audit found
Android does have *a* search box — embedded in `ContactListScreen.kt:154-165` — but it's a
different, weaker mechanism: it hits `GET /contacts?search=`, a naive SQL `LIKE` scan
(`backend/controllers/contact_controller.go:93-99`), not T11's FTS5 `/search` endpoint that web's
`SearchPage.tsx` uses (relation-synonym resolution — "brother" → `sibling_of` — plus notes and
activities coverage, not just contacts). There's also a third, unrelated local FTS4 mirror
(`CachedContactFts.kt`) that only backs offline fallback for cached contact rows — don't confuse
it with this ticket's scope.

## Scope (mirrors `SearchPage.tsx`)

- Replace the placeholder with a real search screen calling the T11 `/search` endpoint.
- Grouped results: Contacts, Notes (with contact chip click-through and "unfiled" indicator),
  Activities.
- Relation-synonym resolution surfaced the way web shows it (`resolved_relation`,
  `SearchPage.tsx:103-107`).
- No-results / min-query-length hints.
- Leave the existing contact-list-embedded naive search bar as-is — it's a reasonable quick filter
  for a page you're already on; this ticket is about giving the *global* search a real backend.

## Done when

- `search` drawer route hits the same FTS5 endpoint and returns the same grouped, synonym-resolved
  results web does for an identical query.
- Notes/Activities results navigate to the right contact.
- Hand-verified on-device with a synonym query (e.g. a relation term) and confirmed against web's
  result set for the same query on the same account.
- New strings translated in all five locales.
