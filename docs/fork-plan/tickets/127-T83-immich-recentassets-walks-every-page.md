# T83 — `RecentAssets` walks a person's entire library to return one asset ⚠

| | |
|---|---|
| **Platform** | Backend |
| **Rating** | 4 — a latent cost that [T70](114-T70-immich-sync-400-diagnosis.md)'s fix has just made reachable for the first time |
| **Size** | S–M — depends entirely on whether Immich's result order can be trusted (item 1) |
| **Depends on** | [T70](114-T70-immich-sync-400-diagnosis.md) (done). **Consider this a release gate for T70** — see below. |
| **Status** | **DONE** (2026-08-12) — see the landing note at the bottom. Item 2 chosen: order verified trustworthy from the deployed version's source. |
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

---

## Landing note — 2026-08-12

**Item 2 chosen: the result order is trustworthy, so the walk can stop early.** The fork was decided
from the source of the exact deployed version rather than a live capture. In Immich v3.1.0 (the
version this integration pins, per T59) `SearchRepository.searchMetadata` orders by
`asset.fileCreatedAt` DESC by default (`orderDirection` defaults to `desc`), and the HTTP
`MetadataSearchDto` accepts an explicit `order` (`"desc"`/`"asc"`) with the sort field fixed
server-side to fileCreatedAt — there is no way to request createdAt ordering. A `LOG_LEVEL=debug`
capture was not needed (and could not have shown ordering anyway: `doRequest` logs
method/url/status on success, never response bodies); the client now sends `order: "desc"`
explicitly, so the early-stop never depends on the server's default changing.

### What changed

`RecentAssets` (`services/immich_client.go`):

- Requests **`size: limit`** instead of the hardcoded 200 — `contactImmichSummary` now asks for
  exactly the one asset it renders, not a 200-asset page.
- Sends **`order: "desc"`** explicitly.
- **Stops the pagination loop as soon as `limit` assets are in hand**; the safety-net re-sort only
  ever reorders what was fetched.
- The 100-iteration cap is now a named constant, `maxImmichSearchPages`, documented as a pure
  non-termination guard, not a correctness knob.
- `limit` is clamped to ≥ 1 defensively (the endpoint rejects `size: 0`).

### Request counts, before → after

| Call site | Limit | Before | After |
|---|---|---|---|
| `contactImmichSummary` (contact page) | 1 | up to 100 requests / 20,000 assets | **exactly 1** |
| `syncImmichPerson` (background sync) | 25 | up to 100 | **exactly 1** |
| photo picker | 30 | up to 100 | **exactly 1** |

One request answers every call whenever the person has at least `limit` assets — the common case.
The loop only continues past page 1 if the server pages smaller than the requested size.

### The approximation, stated plainly

"Latest appearance" now means newest by `fileCreatedAt`, the same field the server orders by. The
only divergence is a photo with no parseable taken date (no EXIF): the server pages it by its DB
fileCreatedAt while the client's `assetOccurredAt` fallback to createdAt could rank it higher — an
asset like that deeper than the fetched window is missed. Real Immich photos always carry
fileCreatedAt, so in practice the two orders agree. Documented in `RecentAssets`' doc comment.

**Also corrected while in here:** the `ImmichAsset` field comment claimed "Latest appearance is best
approximated by the newer of the two (fileCreatedAt, createdAt)" — but `assetOccurredAt` always
preferred fileCreatedAt. The comment now describes what the code actually does. This makes the
early-stop even safer than the ticket's worst case, which assumed a real `max()` sort that could
genuinely disagree with the server's order; it cannot.

### Test coverage

The fake server now models the real endpoint's contract for `size`/`order`: it honors the requested
size (with the `PageSize` knob overriding to force smaller-than-requested paging, which is what
makes the early-stop observable), returns items in fileCreatedAt-desc order, and records
page/size/order per request.

- `TestImmichClient_RecentAssetsRequestCountBoundedByLimit` — **the ticket's request-count pin.**
  limit 1 against a 100-asset person paged 10/page → exactly 1 request (was a 10-page walk);
  limit 25 → stops after pages `[1 2 3]`, not all 10. Also pins `size` = limit and `order` = "desc".
- `TestFetchImmichPersonSummary_RecentAssetsIsASingleRequest` — the service-level measure: a contact
  summary for a 50-asset person is exactly one search request, asking for size 1.
- `TestImmichClient_RecentAssetsSafetyNetSortOnDivergence` — pins the safety-net sort over the
  fetched set (a no-EXIF asset's createdAt fallback reorders it ahead) and the limit-1 approximation
  (that same asset, paged last by an empty fileCreatedAt, is missed).
- Existing pagination/ordering tests updated to the new semantics; the T70 pagination walk and the
  unusable-page-token tests are unchanged and still pass.

**Hand-verified per `/CLAUDE.md`:** neutering the early-stop, and separately restoring `size: 200`,
each made the request-count/size assertions fail; both restored and re-confirmed green.

A Playwright e2e was added (`immich.spec.ts`): a *linked* person's contact page renders its Immich
row from cached metadata without crashing — the exact surface `contactImmichSummary`'s limit-1 call
serves. The e2e environment has no reachable Immich, so request counts are pinned by the Go tests
against the real-protocol fake, not the browser.

`go build ./... && go vet ./... && gofmt -l . && go test ./...` clean; `npx tsc --noEmit` and
`npx vitest run` green; full Playwright suite green (136 tests).

**T70's release gate is now closed**: with both the pagination fix and this bounding fix in, a
contact-page load for a well-photographed person makes exactly one Immich request, not up to 100.
