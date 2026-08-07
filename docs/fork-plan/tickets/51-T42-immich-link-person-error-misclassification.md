# T42 — Immich "link a person" fails with "Could not reach Immich" even when the instance is up and reachable

| | |
|---|---|
| **Rating** | 4 — fully blocks the "link a person" step of an already-shipped, real-data feature |
| **Size** | S |
| **Depends on** | [T15/T16](33-T15-T16-immich.md) (done — owns the client/controller this fixes) |
| **Alpha** | n/a — real data exists, but this is error-handling/logic only: no schema change |
| **Source** | Real-world usage report, 2026-08-06: Settings' "Test connection" succeeds, but opening the person picker from a contact fails with `Immich service error: Could not reach Immich. Is the instance up? (ref: …)` |

## Why this exists

Settings' "Test connection" (`Ping` + `GetMyUser`) passes — the instance is up, the URL is right,
the API key is valid. But opening the "link a person" picker from a contact fails immediately
with the generic unreachable message. That's not possible if the two calls hit the same host with
the same client — and they do (`buildImmichClient`, `immich_service.go:174-191`, is shared by
`TestImmichConnection` and `ListImmichPeopleForUser`). The divergence isn't the URL; it's that the
two code paths ask Immich to do different amounts of work, and only one of them can fail with a
real, non-network error from Immich itself:

- Test connection: `GET /api/server/ping` and `GET /api/users/me`
  (`immich_client.go:247-268`) — trivial, parameter-free, near-guaranteed 200 on any reachable,
  correctly-keyed instance.
- Link-a-person's people picker: `GET /api/people?withHidden=false&size=500&page=N`
  (`immich_client.go:224`) — parameterized, paginated, and the one call in this integration that
  can plausibly return a non-2xx *application* error (a rejected `size`/`withHidden` param on some
  Immich version, a proxy timeout on a large library, a 5xx) from an instance that is otherwise
  completely healthy.

`do()`'s status switch (`immich_client.go:181-193`) only special-cases `200`, `401`/`403`, and
`404`/`410`. Everything else — including a real 400/422/5xx *response from Immich* — falls into
the `default` branch and is wrapped in the exact same `ErrImmichUnreachable` sentinel used for an
actual dial/connect failure:

```go
default:
    resp.Body.Close()
    return nil, fmt.Errorf("%w: Immich returned %s", ErrImmichUnreachable, resp.Status)
```

The controller then renders every non-auth/not-found/invalid-URL failure through the same generic
fallback (`immich_controller.go:21-32`):

```go
default:
    apperrors.AbortWithError(c, apperrors.ErrExternal("Immich", "Could not reach Immich. Is the instance up?").WithError(err))
```

So a real "Immich responded, but rejected this specific request" is indistinguishable in the UI
from "Immich is down." That's actively misleading — it tells the user to go check whether their
server is running when the server is fine and already proved so via Test Connection.

## What to build

1. **Narrow `do()`'s default case in `immich_client.go`.** Distinguish "no HTTP response at all"
   (the `c.client.Do(req)` error branch, already correctly `ErrImmichUnreachable`) from "Immich
   responded with a status we don't have a sentinel for." The latter is not unreachability — give
   it its own sentinel, e.g. `ErrImmichRequestFailed`, that carries the real status code and (bounded)
   response body for logging.
2. **Log or surface the real status/body** on that new sentinel path — at minimum a `Debug`-level
   log entry (this file already does exactly that pattern at `immich_client.go:180`), ideally
   folded into the error message the controller renders so the user sees something like "Immich
   returned an error (400 Bad Request)" instead of the network-down message.
3. **Update `abortImmichServiceError`** (`immich_controller.go:21-32`) to map the new sentinel to
   its own message/status (502 or 503 is both defensible; pick one and be consistent with the
   existing `ErrExternal` usage), distinct from the "could not reach" text reserved for a genuine
   dial failure.
4. **Root-cause the actual `/api/people` request** against a real Immich instance once the real
   status is visible (this ticket can't diagnose the specific 4xx/5xx blind — get the real status
   first, then decide whether `size=500` or `withHidden=false` needs adjusting for the Immich
   version in question, or whether it's transient/proxy-related).

## Traps

- Don't collapse the distinction the other direction — `ErrImmichUnreachable` must stay reserved
  for `c.client.Do(req)` actually failing (DNS, connect refused, TLS, timeout). Only the "got an
  HTTP response but didn't like the status" case moves to the new sentinel.
- `ListPeople`'s pagination loop (`immich_client.go:214-241`) calls `do()` once per page — a
  mid-pagination failure on page 2 should surface the same way as a page-1 failure, not silently
  return a partial list.
- Keep the `x-api-key` header out of any new logging (existing comment at `immich_client.go:161-166`
  already establishes this convention for the file).

## Done when

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` green.
- A table-driven test on `do()` (or an integration test with a stub HTTP server) proves: a genuine
  transport error still maps to `ErrImmichUnreachable`; a 400/422/500 response maps to the new
  sentinel with the real status preserved; 401/403/404/410 behavior is unchanged.
- A controller test proves `ListImmichPeople` returns a distinct error message for a stubbed 400
  response vs. a stubbed connection failure.
- Hand-verified against a real Immich instance: reproduce the original bug's actual upstream status
  code, confirm the fix surfaces it instead of the generic "is the instance up?" message, and
  confirm Test Connection and the people picker now agree when both should succeed.
- All 5 locale files have real translations for any new user-facing message text.

## Landing note (2026-08-07)

Items 1–3 landed on `feature/t42-immich-link-error-misclassification`: `do()` now returns a new
`ErrImmichRequestFailed`/`ImmichRequestError` (real status + bounded body) instead of collapsing
every non-2xx response into `ErrImmichUnreachable`; `abortImmichServiceError` and
`diagnoseImmichConnectionFailure` both surface it with a distinct message. Table-driven client
tests pin the status classification (401/403/404/410 unchanged, 400/422/500 → the new sentinel,
transport failure still → `ErrImmichUnreachable`) and that a mid-pagination failure still surfaces
instead of returning a partial list; a controller test pins that a stubbed 400 and a stubbed
connection failure now render distinct messages. No frontend/locale changes needed — the frontend
renders the backend's message string verbatim and no locale file mirrors it.

Item 4 (root-causing which parameter the reporting user's real instance actually rejects) and the
real-instance hand-verify bullet above were **deliberately skipped** — no real Immich instance was
available to test against in this session, and the user chose to close the ticket on the strength
of the automated tests rather than block on it. If the original "Could not reach Immich" report
recurs, `LOG_LEVEL=debug` will now log the real upstream status/body, which is what item 4 needs.
