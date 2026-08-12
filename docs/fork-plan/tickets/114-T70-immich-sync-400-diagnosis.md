# T70 — Immich sync reports a 400 error

| | |
|---|---|
| **Platform** | Backend |
| **Status** | **DONE** (2026-08-12) — see the landing note at the bottom. Hypothesis #1 confirmed exactly. |
| **Rating** | 4 — same class as T59: a shipped, real-data integration not working end-to-end |
| **Size** | S–M (diagnosis-led, per T59's precedent — the fix itself may be small once the real cause is seen) |
| **Depends on** | [T59](78-T59-immich-v041-still-broken.md) (done — established the debug-logging diagnostic trail this ticket reuses) |
| **Source** | Testing notes, 2026-08-11: "Immich last sync is reporting a 400 error" |

## Why this exists

T59 root-caused and fixed three Immich v3.x mismatches (HTTP/2 stale-session reuse, `/api/people`
response-shape, `GET /api/people/:id/assets` removed) and end-to-end verified connect, browse/link,
summary+thumbnail, and profile-photo picker against the user's real instance. But its own landing
note flagged one thing explicitly unverified: *"Sync was not explicitly re-verified in this
session... the sync path was not part of the original failure report and is rate-limited on the test
instance."* This 400 report is the sync path's turn.

**Where it's tracked**: `ImmichConfig.LastSyncedAt/LastSyncStatus/LastSyncError`
(`backend/models/immich_config.go:43-45`), written by `recordImmichSyncResult`
(`backend/services/immich_service.go:492-510`) from `syncErr.Error()` — no endpoint/path context,
just the message. Frontend renders it verbatim (`frontend/src/components/ImmichSettings.tsx:163-168`).
This is **not** the T42 misclassification bug recurring — the 400 is correctly surfaced as
`ErrImmichRequestFailed`/`ImmichRequestError`, not disguised as "unreachable." The gap is narrower:
the stored message has no path/body, so pinning the exact cause needs a debug-log capture.

**Every call `syncImmichPerson` makes** (`immich_service.go:425-489`), per linked person:
1. `client.GetStatistics(personID)` → `GET /api/people/:id/statistics` — unlikely culprit, identical
   to the call already proven working via the contact-page summary in T59.
2. `client.RecentAssets(personID, 25)` → `POST /api/search/metadata`, paginated via a `nextPage`
   token loop (`immich_client.go:406-450`) — **candidates, ranked:**

**#1, highest confidence — `RecentAssets` pagination shape on page 2+**
(`immich_client.go:406-417`):
```go
reqBody := map[string]any{"personIds": []string{personID}, "size": 200}
if pageToken != "" {
    reqBody["page"] = pageToken   // the prior response's nextPage token, sent back as-is
}
```
`RecentAssets` always paginates through *every* page before trimming to the caller's limit, so this
branch only fires for a person with >200 assets. The fake test server
(`immich_fake_test.go:186-198`) always returns `"nextPage": ""`, so **this branch has never been
exercised by any test**, and T59's "verified working" calls to `RecentAssets` (limit 1 and limit 30
elsewhere) never hit it either if the person tested had ≤200 photos. If Immich's
`/api/search/metadata` expects `page` as an integer rather than the literal token string, a
large-library person would 400 here — exactly the shape of T59's other three fixes (a version/type
mismatch, not a structural bug).

**#2, medium confidence** — `size: 200` rejected by the user's Immich version, same class of issue
as `/api/people`'s `size=500` that T59 investigated (though that one was a red herring there).

**#3/#4, lower confidence** — a stale/malformed linked-person `ExternalID`, or proxy/rate-limit
rejection under sync's heavier request volume (statistics + full pagination per linked person,
looped) — though rate limiting more typically surfaces as 429/503, a weaker fit for a 400 report.

## What to build

1. **Reproduce with `LOG_LEVEL=debug`** and capture the real failing request: method, URL, status,
   body. `doRequest`'s debug line (`immich_client.go:272-273`) already logs all four on any
   unexpected status — it is emitted at **debug** level, so it is invisible at the default level,
   which is why the stored `LastSyncError` has no context.

   Capture recipe — restart the backend with `LOG_LEVEL=debug`, trigger a sync from Immich settings,
   then filter for the line:

   ```bash
   docker compose logs backend | grep "Immich API request: unexpected status"
   ```

   That yields `method`, `url`, `status`, and the response `body` for the failing call. Check
   existing logs from around the original report first — if the instance was already running at debug
   level, the evidence may not need a fresh sync. Ideally reproduce with a linked person whose
   library exceeds 200 assets, to force the pagination branch (#1 below).
2. **Fix whatever the real status/body identifies** — likely the `RecentAssets` page-token type/shape
   (#1) or the `size` cap (#2), following the same "see the real response before guessing" discipline
   T59 used.
3. **Re-verify sync end-to-end** against the user's real instance, not just the previously-verified
   surfaces — this is the one thing T59 explicitly left unverified.

## Traps

- **Don't guess the fix without seeing the real status/body first** — T59's own note calls out that
  guessing repeats T42's mistake in reverse. `LOG_LEVEL=debug` is the established tool.
- **`ErrImmichUnreachable` stays reserved for genuine dial failures** — a status code, even an
  unexpected one, is `ImmichRequestError` territory, same convention as T59.
- Keep `x-api-key` out of any new logging (existing convention at `immich_client.go:161-166`).

## Done when

- The real failing request + status/body is recorded in this ticket's landing note.
- A full sync run completes without error against the user's real instance (`immich.brunning.us` or
  current equivalent), including at least one linked person with >200 assets if the pagination
  hypothesis is confirmed.
- `go build ./... && go vet ./... && gofmt -l . && go test ./...` green.

---

## Landing note — 2026-08-12

**Hypothesis #1 was correct, and the debug capture pinned it in one pass.** The user ran the sync
with `LOG_LEVEL=debug` against the real instance (`immich.brunning.us`); the log shows the first
`POST /api/search/metadata` returning **200** and the immediately following one returning **400**,
which is the pagination branch and nothing else:

```json
{"status":400,"body":"{\"message\":\"Validation failed\",\"errors\":[{\"expected\":\"number\",
\"code\":\"invalid_type\",\"path\":[\"page\"],
\"message\":\"Invalid input: expected number, received string\"}]}"}
```

### The real cause

Immich serializes `assets.nextPage` as a JSON **string** (`"2"`), but `/api/search/metadata`'s
request validator requires `page` to be a **number**. `RecentAssets` decoded the token into a
`string` field and echoed it back verbatim, so page 2 onward was always rejected. An asymmetric API
contract, not a structural bug — the same shape as all three of T59's fixes.

Because the first page omits `page` entirely, this only ever fired for a person with **more than 200
assets**. That is why T59 verified Immich end-to-end without hitting it, and why nothing caught it:
the fake server's `handleSearchMetadata` always returned `"nextPage": ""`, so **the pagination branch
had never been executed by any test**, exactly as this ticket predicted.

`size: 200` is exonerated — the first request carries it and returns 200. Suspects #2/#3/#4 are ruled
out.

### The fix

- `parseImmichPage` (`services/immich_client.go`) reads `nextPage` from a `json.RawMessage` and
  accepts either a JSON string or a bare number, returning the page as an `int`. The request now
  always sends a number.
- Tolerating both shapes on the way *in* is deliberate: the previous `NextPage string` field would
  have failed the whole sync with `ErrImmichInvalidData` had a version ever sent a bare number.
- A token that cannot be a number (an opaque cursor, `null`, `0`) **stops** the loop and returns the
  assets gathered so far, rather than erroring. Since the endpoint requires a numeric `page`, such a
  token is unusable under any encoding — failing the person's entire sync over it would be worse.

### Test coverage added

The fake server now models the real contract on both sides: it **rejects a string `page` with
Immich's actual 400 validation body**, and serializes `nextPage` as a string on the way out, so the
asymmetry is reproduced rather than papered over. Pagination is opt-in per fake person
(`PageSize`), leaving every pre-existing single-page test untouched.

- `TestImmichClient_RecentAssetsPaginatesAcrossPages` — 5 assets at 2/page; asserts all 5 come back,
  still sorted newest-first across pages, and — via the new `SearchPages` recorder — that the client
  actually walked pages `[1 2 3]`. Asserting the pages requested matters: a loop stuck on page 1
  would still return assets and pass a naive length check.
- `TestImmichClient_RecentAssetsStopsOnUnusablePageToken` — opaque cursor, `null`, and `0` each stop
  cleanly and return what was gathered.

**Hand-verified per `/CLAUDE.md`**: reverting the fix to send `strconv.Itoa(pageNum)` made
`TestImmichClient_RecentAssetsPaginatesAcrossPages` fail with the same `400 Bad Request` seen in
production; restored and re-confirmed green.

`go build ./... && go vet ./... && gofmt -l . && go test ./...` all clean.

### Found while reviewing this fix — read before deploying

Two things surfaced in the pre-commit review:

1. **A non-advancing `nextPage` would have duplicated assets.** The first version of the fix tracked
   the implicit first page as `pageNum = 0`, so a server answering the first request with
   `nextPage: "1"` passed the "must advance" check and refetched page 1. Now tracked as `pageNum = 1`
   with a separate `sendPage` flag, since the first request *is* page 1 — it just omits the
   parameter. Caught by the added `non-advancing page` case, which failed with 2 copies of the same
   asset before the correction.
2. **This fix removes an accidental circuit breaker — filed as
   [T83](127-T83-immich-recentassets-walks-every-page.md).** `RecentAssets` walks *every* page before
   trimming to the caller's limit, and `contactImmichSummary` calls it with `limit: 1` synchronously
   during a contact-page load. Until now the 400 aborted that walk after two requests for anyone with
   >200 assets, so the full walk has never actually run in production. With pagination working, a
   contact linked to a well-photographed person can trigger up to 100 sequential Immich requests to
   render one thumbnail. **Treat T83 as a release gate for this fix**, or deploy knowing that cost is
   now live.

### Still outstanding

**Not yet re-verified against the live instance** — that is this ticket's third "Done when" item and
the one thing T59 was faulted for leaving open. Deploy and run a real sync (ideally for the person
with >200 assets, `d69157ef-4dc1-4ea2-8cb4-f636a719a33a` in the capture) before considering this
closed.
