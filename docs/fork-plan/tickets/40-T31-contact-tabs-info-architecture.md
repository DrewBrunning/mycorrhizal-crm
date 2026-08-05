# T31 — Contact detail page: break tabs into grouped cards

| | |
|---|---|
| **Rating** | 4 — the contact detail page is the most-used surface, and its own tabs are hard to reach |
| **Size** | M |
| **Depends on** | — (complements [T28](21-T28-mobile-contact-layout.md), which fixed the tab bar's *mobile* scrolling; this fixes the underlying *count* of tabs) |
| **Alpha** | n/a — real data exists; this is a frontend reorganization, no schema change |
| **Source** | v0.2.0-alpha real-world testing |

## Why this exists

`ContactDetailPage.tsx`'s tab bar has grown past what a horizontal tab strip comfortably holds —
General Information, Relationships, Life Events, Preferences, Gifts, Conversation Agenda,
Connections/Graph, Cadence, and whatever else has accreted since. Even with T28's horizontal
scroll fix, the practical effect on a normal (non-mobile) viewport is that only ~4 tabs are
reachable without scrolling or hunting — the tab bar has stopped being a good navigation
affordance at this width.

The reporter's own suggestion, and the one this ticket adopts: Monica handles this by using more
cards on a single scrollable page rather than more tabs, grouping tightly related concepts
together. That trades "which tab is it under" for "scroll down," which degrades much better as
the number of sections keeps growing (it already has, several times, since this page was
designed).

## What to build

Restructure `ContactDetailPage.tsx` from a tab-per-concept layout into a small number of anchor
sections (cards) on one scrollable page, grouping tightly related concepts together rather than
one tab each. A reasonable grouping, subject to revision once laid out for real:

- **Overview** — header, General Information, Preferences, custom fields
- **People** — Relationships, Household membership, Connections/graph panel
- **Timeline** — Notes, Activities/Interactions, Life Events, Conversation Agenda (things already
  presented as a merged timeline elsewhere, per `91.8`, may belong together here too)
- **Cadence & follow-up** — Cadence panel, upcoming reminders
- **Gifts**
- **External links** — Immich, other `ExternalIdentity` panels

Keep a short in-page jump nav (anchor links or a sticky mini-menu) so a user who wants to go
straight to "Relationships" doesn't have to scroll past everything else — this is what keeps the
card approach from just being "one very long page" with the same findability problem tabs had.

On mobile, this should degrade to what T28 already built (scrollable/collapsible sections) rather
than reintroducing a tab bar — the point is not to have two different navigation metaphors for
desktop vs. mobile.

## Traps

- This touches nearly every section currently gated behind a `<Tab>` — expect broad, low-risk
  changes across `ContactDetailPage.tsx` and possibly `ContactInformation.tsx`'s internal
  structure. Do it as a genuine reorganization, not a wrapper that keeps the tab state model and
  just changes how it's drawn — that would keep the underlying problem (a `activeTab` piece of
  state gating visibility) instead of removing it.
- Field-visibility settings (`contactFields.ts`) currently likely key off which tab a field
  belongs to for some organizational purpose — check for any code that assumes a tab structure
  (e.g. tab-scoped "hide this tab entirely" settings) before removing tabs outright.
- [T30](39-T30-hide-empty-subtitles.md) should land first or alongside this — an empty card with
  a visible header is the exact same bug T30 fixes, now at the level of whole cards instead of
  subsections.
- Existing Playwright e2e specs almost certainly click into named tabs (`getByRole('tab', {name:
  ...})` or similar) — expect to update selectors across the contact-detail e2e suite, not just
  component tests.

## Done when

- The contact detail page has no more than a handful of top-level navigation destinations
  (anchors/cards, not tabs), each grouping related concepts.
- Every concept previously reachable via a tab is still reachable, with no additional clicks
  required to discover it exists (i.e., not hidden behind an unlabeled "more" affordance).
- Mobile behavior matches T28's existing patterns rather than introducing a second navigation
  style.
- `npx tsc --noEmit` clean, `npx vitest run` green, e2e contact-detail specs updated and green.
- Visually verified at desktop and mobile widths.
