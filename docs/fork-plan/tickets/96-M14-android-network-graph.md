# M14 — Network graph on Android

| | |
|---|---|
| **Rating** | 3 — real design work, not a port; T10's graph on a 6" screen is a genuinely different problem |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | Nothing — T10's graph traversal backend already exists and serves web today |
| **Status** | TO BE DONE |

`MycorrhizalApp.kt:498` wraps the `"network"` drawer route in `PlaceholderScreen`; no graph
implementation exists anywhere in `android/`. M8's own ticket flagged this as "the clearest case" of
a web surface that shouldn't be a straight port — a force-directed graph with drag/zoom/pan doesn't
translate to touch well, and the sign-off target still calls for it to be built (no exclusion was
recorded for this one), so the deliverable is a **mobile-appropriate design**, not a pixel port of
`NetworkGraph.tsx`.

## Scope — design first, then build

1. **Design pass required before implementation.** Web's interaction model
   (`components/NetworkGraph.tsx:331-400`: drag nodes, hover tooltips, pan/zoom canvas) assumes a
   mouse and a large viewport. Candidates worth evaluating: a simplified force-graph with
   tap-to-select instead of hover, a list/tree view of 1-hop and 2-hop relationships as a fallback
   or primary mode, or a radial "this contact + connections" view centered on whoever you navigated
   from. Don't default to porting the desktop interaction model uncritically.
2. Once a design is picked, cover the same underlying capabilities web has: click contact node →
   navigate to contact; click activity node → view/edit that activity; filter by contact
   (`NetworkPage.tsx:208-219`) and by circle (`:221-233`); toggle relationships/activities/circles
   visibility (`:235-266`); legend.

## Done when

- A design decision is written down (in this ticket's landing note) before implementation starts.
- Contact and activity nodes are reachable and actionable on a phone-sized viewport without desktop
  mouse affordances (hover, drag-to-pan) as the only way in.
- Filter-by-contact and filter-by-circle both work.
- Hand-verified on-device against a contact with a non-trivial relationship graph.
- New strings translated in all five locales.
