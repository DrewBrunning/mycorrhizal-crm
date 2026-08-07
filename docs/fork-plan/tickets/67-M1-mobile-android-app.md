# M1 — Native Android app (Kotlin, Jetpack Compose)

| | |
|---|---|
| **Rating** | 2 — "in the future," but more solidified than the Feature ideas bucket; a real, intended-to-happen project |
| **Alpha** | after — gated on API contract stability, not on this app's own alpha/beta line |
| **Source** | v0.3.0 post-release planning, 2026-08-06 |

## Why this exists

A native Android app can reach into things the browser fundamentally can't: call log, SMS, and the
device's own contacts — which is what makes automated interaction tracking possible (a real call or
text logged as a cadence-qualifying `Activity` without the user manually entering it, per `91.10`'s
qualifying-interaction rule). That's the actual reason for a native client rather than a better
mobile web experience; PWA install already covers "an icon on the home screen," which this project
already has (N9's service worker work).

**Not implementation-ready — deliberately not scoped yet.** This is filed to record the intent and
its real gate, not to describe what to build. A design pass is a separate, later ticket.

## What's already decided

- **Kotlin + Jetpack Compose** for the UI.
- **Codegen'd API client from the OpenAPI spec** ([T8](16-T8-openapi.md), done — the drift-tested
  `backend/openapi.yaml` this depends on) rather than a hand-written HTTP layer. This is the concrete
  reason T8/[T12a](14-T12a-etag-primitives.md)/[T17](17-T17-change-feeds.md) were rated up from 2 to
  4 pre-alpha on the strength of "a mobile client might need this" (see `tickets/README.md`'s own
  footnote on that rating decision) — this ticket is that consideration finally becoming concrete
  rather than speculative.
- **Automated contact-interaction tracking** (call/text history → `Activity` entries) is the headline
  capability, not just "the app but native."

## The real gate

**Not ready to start before the API surface is solid** — this app is pre-alpha/alpha, and generating
a client against a contract that's still moving means regenerating and re-verifying the mobile app
on every backend change. Earliest realistic entry point is **the move from beta to a real v1.0.0**,
per the source conversation — not before.

Two existing tickets are concretely gated on this one, not the other way around:
- [T57](66-T57-bulk-import-api-for-external-clients.md) — a documented bulk-import API a mobile
  client could drive to pull in the device's full contact list on first setup. A direct sub-piece of
  this initiative, not a separate idea.
- [P4](68-P4-local-model-pilot.md) — the local-model code-gen pilot re-enters scope specifically
  "when mobile client work begins," per its own note.

## Open questions for the eventual design pass

- Call/SMS access requires Android runtime permissions with real user trust implications — how much
  is read locally and summarized vs. sent to the server, and what's the user-facing consent/control
  surface?
- Does automated interaction tracking write `Activity` rows directly, or land as a
  propose-then-approve suggestion (matching the household-inference pattern, `91.4`) the way
  [P3](76-P3-ai-ollama-layer.md)'s L4–L5 outputs are required to? Automatically logging a real phone
  call is a stronger claim than a household guess — leaning toward suggestion-first is likely right,
  but that's a decision for the design pass, not this ticket.
- Auth model for a long-lived native client (the web app currently uses a cookie-based session, per
  `README-developer.md`) — token-based auth for the mobile client is the obvious shape but needs its
  own design pass, not an assumption baked in here.

## Done when

N/A — not scheduled. This ticket exists to record the plan and its gate. Re-evaluate (and split into
a real, scoped design-pass ticket) once the API surface is judged stable enough — the move to
v1.0.0 is the earliest point to reconsider, not a commitment to start then.
