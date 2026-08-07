# T51 — Browser push "Test notification" fails with 413 from the push service

| | |
|---|---|
| **Rating** | 4 — breaks a recently-shipped real feature (N9 browser push) for a realistic configuration, not a corner case |
| **Size** | S |
| **Depends on** | [N9](30-N9-notification-channels.md) (done — owns the code this fixes) |
| **Alpha** | n/a — real data exists; server-side dispatch logic only, no schema change |
| **Source** | v0.3.0 post-release testing, 2026-08-06 — "unexpected status 413 from push service," reproduced on Firefox desktop (Ubuntu) and Firefox Android; registration itself succeeds |

## Why this exists

Registration works (confirmed: the subscription is created and stored), but sending a test message
fails at delivery. Traced to `sendPushMessage` (`services/notification_service.go:369-403`), which
calls `webpush.SendNotification` (`github.com/SherClockHolmes/webpush-go v1.4.0`) with an
`Options{}` struct that never sets `RecordSize`.

`webpush-go`'s encryption step (`webpush.go:160-186`) always pads the plaintext out to the *full*
record size before encrypting — not to the size the actual message needs:

```go
recordSize := options.RecordSize
if recordSize == 0 {
    recordSize = MaxRecordSize   // 4096
}
...
// Pad content to max record size - 16 - header
if err := pad(dataBuf, recordLength-recordBuf.Len()); err != nil { ... }
```

`MaxRecordSize` is `4096` (`webpush.go:23`). Since this app never overrides `RecordSize`, **every**
push notification — including a two-word test message — gets padded to a ~4096-byte plaintext
before encryption, producing a request body of roughly 4180+ bytes (salt + record-size header +
public key + ciphertext + AEAD tag) regardless of actual content length. The failure being
reported cross-platform (Firefox desktop, Firefox Android) rather than on one specific device
points at the push service enforcing a smaller limit than this fixed padding produces, not a
browser-specific quirk — consistent with this being a payload-size problem, not a registration or
subscription-validity one.

## What to build

1. **Set `webpush.Options.RecordSize`** in `sendPushMessage` to something sized to the actual
   payload rather than relying on the library's 4096-byte default — e.g. based on
   `len(payload)` plus the fixed encryption overhead, with a sane floor/ceiling, rather than always
   padding to the maximum. `payload` here is the small `{"title":..., "body":...}` JSON already
   being marshaled at `notification_service.go:370` — there's no reason it should ever produce a
   multi-KB encrypted body.
2. Confirm the same applies to the real reminder-delivery path (`Send` at
   `notification_service.go:405` on), not just the test-button path — they share `sendPushMessage`,
   so fixing the one function fixes both, but verify both are actually exercised in testing.

## Traps

- This can't be fully verified against a mock — the whole point is the push *service's* real
  behavior (Mozilla's autopush, or whichever Google/other endpoint the reporting devices used), not
  this app's own code. `N9`'s own ticket states the same bar for its other channels ("Verified end
  to end against a real ntfy or Gotify instance, not a mock") — this needs the equivalent for push:
  a real browser subscription against a real push service, not a stubbed HTTP response.
- Don't shrink `RecordSize` to a single fixed small constant either — Web Push messages vary in
  length (a reminder digest line is longer than "Test notification"), and an under-sized record for
  a longer message could truncate or fail a different way. Size it to the actual content.
- Re-check whether stale/failed push subscriptions get incorrectly marked as needing pruning as a
  side effect of a 413 being misread as an endpoint-gone signal — `sendPushMessage`'s status switch
  (`notification_service.go:396-402`) only treats `404`/`410` as stale; confirm 413 correctly falls
  through to the generic error path (it does, per current code) and stays that way after the fix.

## Done when

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` green.
- Hand-verified against a real browser push subscription and a real push service (the same
  Firefox/Android or Firefox/Ubuntu setup that reproduced the bug) — confirm the test notification
  actually arrives, not just that no error is returned.
- A unit test on `sendPushMessage`/its `RecordSize` computation pins that a short payload no longer
  gets padded to the library's 4096-byte default.
