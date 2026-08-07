# T57 — Documented/stable bulk-import API for external clients (deferred)

| | |
|---|---|
| **Rating** | 1–2 — no concrete consumer exists yet; re-evaluate when one does |
| **Size** | unscoped — needs its own design pass once there's a real client to design against |
| **Depends on** | [T56](65-T56-bulk-contacts-import-flow.md) |
| **Alpha** | n/a |
| **Source** | v0.3.0 post-release testing, 2026-08-06 — raised alongside T56: "when we make an Android app, having an API for it that works will let us import the entire set of contacts outright" |

## Why this exists

T56 gives the in-app UI a way to bulk-import. This is the separate question of whether the
import machinery (`ParseVCF`/`ParseCSV`, the session/preview/confirm flow) should be exposed as a
*documented, stable* API contract — something an external client (a future Android app, most
concretely) could drive directly rather than a human clicking through the Data Settings UI. That's
a materially different bar: a UI-backing endpoint can change shape whenever the UI does; an API a
mobile client depends on needs versioning discipline and can't casually break.

## Why it's deferred, not ticketed for real

There's no concrete consumer today — no Android app exists yet. Per this repo's own precedent
([P1b/P2/P3/P4](37-deferred.md)), a speculative integration surface without a real client to design
against isn't a sizeable ticket, it's a placeholder for a design pass that should happen once the
actual consumer (and its actual constraints — auth model, expected batch sizes, offline/retry
behavior) exists. Scoping this now would mean guessing at requirements a real mobile client would
immediately contradict.

## Pulled in when

A concrete external client (the Android app, or anything else) is actually being built and needs
this. At that point this ticket becomes a real design pass: what auth a mobile client uses, whether
the existing session-based preview/confirm flow (built for one browser tab, one user, synchronous)
is the right shape for a mobile client's bulk-import UX, and how the API version story works
alongside `T8`'s OpenAPI coverage.

## Done when

N/A — not scheduled. Re-evaluate this ticket's rating/size once a real consumer exists.
