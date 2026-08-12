# T83 — `RecentAssets` walks a person's entire library to return one asset ⚠

| | |
|---|---|
| **Platform** | Backend |
| **Rating** | 4 — a latent cost that [T70](114-T70-immich-sync-400-diagnosis.md)'s fix has just made reachable for the first time |
| **Size** | S–M — depends entirely on whether Immich's result order can be trusted (item 1) |
| **Depends on** | [T70](114-T70-immich-sync-400-diagnosis.md) (done). **Consider this a release gate for T70** — see below. |
| **Status** | TO BE DONE |
| **Source** | Found reviewing T70's fix before commit, 2026-08-12. Not a pre-existing complaint — the bug T70 fixed had been masking this. |

## Why this exists

`RecentAssets` (`services/immich_client.go`) paginates through **every** page of a person's assets —
up to the 100-page safety cap at 200 per page, so as many as 20,000 assets and 100 sequential HTTPS
round-trips — and only then sorts by occurrence and trims to the caller's `limit`.

Its three callers ask for far less:

| Call site | Limit | Context |
|---|---|---|
| `immich_service.go:334` (`contactImmichSummary`) | **1** | **Synchronous, on the contact detail page** — and now inside M4's composite |
| `immich_service.go:430` (`syncImmichPerson`) | 25 | Background sync, per linked person |
| `immich_service.go:602` (photo picker) | 30 | User-initiated |

The full walk is deliberate: the client re-sorts by `max(fileCreatedAt, createdAt)` rather than
trusting the server's ordering, and a correct "newest N" under that assumption genuinely requires
reading everything.

**This has never actually run in production.** For a person with ≤200 assets there is only one page,
so the walk is a single request. For a person with >200 assets, page 2 always failed with the 400
that [T70](114-T70-immich-sync-400-diagnosis.md) just fixed — the loop aborted after two requests and
the caller degraded silently (`contactImmichSummary` does `return summary` on error). So the bug was
acting as an accidental circuit breaker.

**With T70's fix in place, that breaker is gone.** A contact linked to a well-photographed Immich
person will, on every contact-page load, trigger a paginated walk of their whole library to display
*one* thumbnail. The 30 s per-request timeout applies per request, not per call, so the total is
unbounded in practice. Suspect #4 in T70's original triage — proxy or rate-limit rejection under
sync's request volume — also becomes considerably more likely.

## What to build

1. **First, establish whether the result order can be trusted** — do not guess, per the discipline
   T59 and T70 both used. With `LOG_LEVEL=debug` against the real instance, capture a two-page
   response and check whether `/api/search/metadata` returns newest-first, and whether the version in
   use accepts an explicit order parameter. Everything below forks on the answer.
2. **If order is trustworthy** (or can be requested explicitly): stop as soon as `limit` items are in
   hand. `contactImmichSummary` drops from up to 100 requests to exactly one, and the client-side
   sort becomes a cheap safety net over a small set rather than the reason for the walk.
3. **If it is not**: bound the walk with an explicit page cap (a named constant, not the incidental
   100-iteration safety cap) and accept "newest among the first N×200 assets". Document the
   approximation where a reader will find it — "latest appearance" silently meaning "latest among a
   sample" is exactly the kind of quiet inaccuracy this codebase's comments exist to prevent.
4. **Either way, pass the caller's `limit` down.** Requesting `size: 200` to satisfy `limit: 1` is
   wasteful even on a single page.
5. Extend the fake server's pagination (added in T70) with a test asserting the request *count* for a
   given limit, so a future change can't quietly reintroduce the full walk.

## Traps

- **Don't just lower the 100-page cap and call it fixed.** That bounds the damage without addressing
  a contact page making dozens of upstream requests to render one image.
- **`contactImmichSummary` swallows errors** (`return summary`), so a regression here degrades
  silently rather than surfacing — the same property that let T70's 400 go unnoticed on that path.
- **The photo picker genuinely wants 30 spread across a real library**, so a fix tuned only for
  `limit: 1` may not serve it. Check all three call sites.

## Done when

- A contact-detail page load for a person with a large Immich library issues a small, bounded number
  of Immich requests — measured, with the before/after counts in the landing note.
- Whichever of items 2 or 3 was chosen is recorded along with the evidence from item 1.
- A test pins the request count for a given limit.
- `cd backend && go build ./... && go vet ./... && gofmt -l . && go test ./...` green.
