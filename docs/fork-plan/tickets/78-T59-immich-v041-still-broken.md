# T59 — Immich still broken in v0.4.1 testing

| | |
|---|---|
| **Rating** | 4 — a shipped, real-data feature that is still not usable end-to-end after T42 |
| **Size** | S–M (diagnosis-led; the fix itself may be tiny once the real cause is seen) |
| **Depends on** | [T42](51-T42-immich-link-person-error-misclassification.md) (done — classification fix landed, item 4 deliberately skipped) |
| **Alpha** | n/a — real data exists, but this is connection/logic debugging, not a schema change |
| **Source** | v0.4.1 testing, 2026-08-09: "Immich still broken" |

## Why this exists

T42 fixed the *symptom-classification* half of the Immich "link a person" failure — `do()` now
distinguishes "Immich is unreachable" from "Immich answered with a real non-2xx status," and the
controller surfaces the real status instead of the misleading "Could not reach Immich" message. But
T42's **item 4 was deliberately skipped**: the actual `/api/people` request that the reporting user's
real instance was rejecting was never root-caused, because no real Immich instance was available in
that session. T42's landing note explicitly says: "If the original 'Could not reach Immich' report
recurs, `LOG_LEVEL=debug` will now log the real upstream status/body, which is what item 4 needs."

The v0.4.1 testing report that Immich is **still broken** is exactly that recurrence. The
classification improvement landed but the underlying integration does not work for the user. Whatever
the specific symptom (link-a-person picker fails, profile-photo-from-Immich fails, thumbnail/summary
shows nothing, or sync errors), this ticket owns getting it actually working against a real instance,
starting from the diagnostic trail T42 put in place.

## What to build

1. **Reproduce and capture the real upstream response.** With the user's instance and `LOG_LEVEL=debug`
   (or the server logs / the frontend's rendered `ErrExternal` message, which now carries the real
   status), record exactly which request fails, with what status and body. Candidates, in order of
   how likely they are to be the culprit:
   - `GET /api/people?withHidden=false&size=500&page=N` — the link-a-person picker
     (`immich_client.go:264`). `size=500` and/or `withHidden=false` are the prime suspects for a
     version-parameter rejection.
   - The thumbnail/summary calls (`/api/people/{id}/statistics`, `/thumbnail`).
   - The recent-assets call for the profile-photo picker (`ListImmichRecentAssetsForContact`).
2. **Fix whatever the real status identifies** — likely one of:
   - Drop/limit `size`, or switch to the documented per-page cap for the user's Immich version.
   - Correct the person/stats/thumbnail path for the version in question.
   - A proxy/auth/TLS issue surfaced by the now-visible real status.
3. **Re-verify the whole Immich surface end-to-end against a real instance**, not just the one failing
   call: connect (Settings test), browse/link a person, show the contact-page summary + thumbnail,
   pick a profile photo from Immich, and run a sync. T42 only ever pinned the classification; nothing
   has yet proven the happy path works against real Immich.

## Traps

- **This is the continuation of T42's item 4, not a fresh start.** Re-read T42's landing note before
  picking this up — the classification sentinels and debug logging are already in place and are the
  intended diagnostic tool. Do not re-litigate them.
- **`ErrImmichUnreachable` stays reserved for genuine dial failures** — if the real response is a
  status code (even an unexpected one), that is `ImmichRequestError`/`ErrImmichRequestFailed`
  territory, not "the instance is down."
- **Do not guess the version-specific fix.** The whole point is to see the real status/body first
  (`LOG_LEVEL=debug`, `immich_client.go:230-232`). Shipping a guessed `size`/`withHidden` change
  without seeing the rejection repeats T42's exact mistake in reverse.
- **Frontend message strings render the backend's message verbatim** — if a new user-facing message
  is added, it still needs real translations in all five locale files only if the frontend mirrors it;
  the Immich error strings so far are backend-owned and rendered as-is (see T42's landing note).
- Keep the `x-api-key` header out of any new logging (existing convention at `immich_client.go:161-166`).

## Done when

- The real failing request + status/body from the user's instance is recorded in this ticket's landing
  note (or the fix is otherwise proven against a real instance, not just a stub).
- The identified fix is in and the whole Immich surface — connect, browse/link, summary + thumbnail,
  profile-photo picker, sync — works against a real Immich instance.
- `go build ./... && go vet ./... && gofmt -l . && go test ./...` green.
- If a stub/server test was the only feasible verification, the frontend and backend both degrade
  gracefully (no contact page errors) when Immich is down, per T15/T16's traps.

## Landing note (2026-08-09)

Root-caused and fixed against a live Immich v3.1.0 instance. Three independent problems, all
found via `LOG_LEVEL=debug` (exactly as T42's landing note intended):

1. **HTTP/2 stale-session reuse** (`immich_client.go`): the shared `http.Transport` reused an
   HTTP/2 connection opened by Test Connection, but Caddy at the edge had already closed the
   session by the time ListPeople reused it. Forced HTTP/1.1 via `TLSNextProto`, added
   `IdleConnTimeout` (30s), `TLSHandshakeTimeout` (10s), and `ResponseHeaderTimeout` (15s).

2. **`/api/people` response-shape mismatch** (`immich_client.go`): Immich v3.1.0 returns
   `{"people": [{id, name}, …]}` (flat array) while the code expected
   `{"people": {"items": […], "hasNextPage": bool}}`. `ListPeople` now tries the newer shape
   first and falls back to the flat array.

3. **`GET /api/people/:id/assets` removed in v3.x** (`immich_client.go`): Replaced with
   `POST /api/search/metadata` using the `personIds` filter and `nextPage`-token pagination.
   `RecentAssets` paginates through all pages, sorts globally by date, and limits.

Also fixed: `ErrImmichInvalidData` was unhandled in `abortImmichServiceError` and fell through
to the misleading "Could not reach Immich" default — now surfaces a distinct parse-error message
(`immich_controller.go`).

End-to-end verified against the user's production instance (`immich.brunning.us`, Immich v3.1.0):
test connection, browse/link a person, contact-page summary + thumbnail, and profile-photo picker
all work. Sync was not explicitly re-verified in this session (the sync path was not part of the
original failure report and is rate-limited on the test instance).
