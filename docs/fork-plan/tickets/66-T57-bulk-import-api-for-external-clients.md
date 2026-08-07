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

**Not just a first-run flow.** The Android app needs this from at least two separate places in its
own UX, not one: a first-run "would you like to import your contacts?" onboarding prompt, and a
separate, always-available "Import from contacts" entry point in the app's own Data settings
(mirroring where T56 puts the equivalent flow on the web) so a user can re-trigger an import later —
after adding new device contacts, say, not only at setup. Relative to this API, that distinction
shouldn't matter: one stable bulk-import contract serves both call sites identically, the same way
today's single backend endpoint doesn't care whether a browser request came from an onboarding
screen or a settings page. Noted here so a future design pass scopes the API contract itself
correctly (repeatable, not a one-shot "setup wizard" call) rather than only against the narrower
first-run case.

## Why it's deferred, not ticketed for real

There's no concrete consumer today — [M1](67-M1-mobile-android-app.md) records the intent to build
a native Android app, but it hasn't started, and this ticket is a named sub-piece of that work (see
M1's own "what's already decided" section). Per this repo's own precedent for the Deferred section
(`tickets/README.md`'s Deferred table), a speculative integration surface without a real client to
design against isn't a sizeable ticket, it's a placeholder for a design pass that should happen once
the actual consumer (and its actual constraints — auth model, expected batch sizes, offline/retry
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
