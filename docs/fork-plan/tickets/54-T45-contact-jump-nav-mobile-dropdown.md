# T45 — Contact detail jump nav should collapse to a dropdown on narrow viewports

| | |
|---|---|
| **Rating** | 3 — real friction on the most-used page's own navigation, but content stays reachable today (T31's own e2e already pins "no horizontal overflow") |
| **Size** | S |
| **Depends on** | [T31](40-T31-contact-tabs-info-architecture.md) (done — owns `ContactJumpNav`) |
| **Alpha** | n/a — frontend-only, no schema/API change |
| **Source** | Real-world usage report, 2026-08-06: "Top bar in contacts (Overview, People, Timeline, etc) doesn't shrink well - this should become a dropdown on mobile/small (maybe on medium?)" |

## Why this exists

`ContactJumpNav` (`ContactDetailPage.tsx:159-194`) renders its six section links
(Overview/People/Timeline/Cadence & follow-up/Gifts/External links) as a single row of `Button`s
in a `Box` with `overflowX: 'auto'` (`ContactDetailPage.tsx:168-170`):

```tsx
sx={{
  position: 'sticky',
  top: { xs: 56, sm: 64 },
  zIndex: 10,
  display: 'flex',
  gap: 0.5,
  overflowX: 'auto',
  ...
}}
```

T31 deliberately chose this over a second mobile-specific nav pattern ("the same jump-nav pattern
is used on desktop and mobile... rather than two navigation metaphors" — its own landing note), and
`e2e/contactDetailLayout.spec.ts` pins "no horizontal overflow at 390px." That test is checking the
row doesn't visually break the page, not that it's a good navigation affordance at that width — a
horizontally-scrolling button strip with no visible "there are more items" cue is exactly the kind
of control that reads as broken/incomplete on a phone, which is what's being reported here.

The app already has an established "mobile" breakpoint for exactly this kind of collapse:
`theme.breakpoints.down('md')` is used as `isMobile` in `App.tsx:88` and `NetworkGraph.tsx:51`
(`down('sm')` is used elsewhere, e.g. `ContactHeader.tsx:107`, for a *tighter* "compact" cutoff on
top of that). Matching `down('md')` here answers the reporter's own "maybe on medium?" question —
it's the codebase's existing mobile cutoff, not a new one-off threshold for this component.

## What to build

- Below `theme.breakpoints.down('md')`, replace the button row with a `Select` (or a `Menu`
  triggered by a single button showing the current/nearest section) listing the same six sections,
  `onChange` navigating to `#${id}` the same way the button `href`s do today.
- Above that breakpoint, keep the existing horizontally-scrolling button row unchanged — this is
  additive, not a rewrite of desktop behavior.
- Decide whether the dropdown should track and reflect the currently-visible section (an
  IntersectionObserver per `SectionGroup`, mirroring the deferred approach `T31`'s landing note
  already flagged for the Connections panel) or just act as a plain jump menu with no active-state
  tracking. A plain jump menu is the smaller, lower-risk option and is enough to resolve the
  reported complaint — treat live tracking as optional polish, not a blocker.

## Traps

- `ContactJumpNav`'s `sx.top` already accounts for the AppBar height (`{ xs: 56, sm: 64 }`) and
  `SectionGroup`'s `scrollMarginTop: 112` (`ContactDetailPage.tsx:136`) accounts for the nav's own
  height on top of that — a dropdown trigger needs the same or smaller height than the current
  button row, or `scrollMarginTop` needs adjusting to match, or anchor scrolling will land under
  the sticky nav again (the exact failure class `ContactDetailPage.tsx:474-485`'s comment already
  describes for a different case).
- `e2e/contactDetailLayout.spec.ts` almost certainly asserts against the button-row structure at
  narrow widths — update it for the new dropdown, don't just relax the overflow assertion.
- Keep the `aria-label` (`ariaLabel={t('contactDetail.jumpNav')}`) and general accessibility
  posture — a `Select`/`Menu` needs the same "what is this control for" affordance the nav landmark
  already provides.
- i18n: no new section labels are needed (reusing existing `contactDetail.section.*` keys), but any
  new "jump to section" trigger label needs real translations in all 5 locales.

## Done when

- `npx tsc --noEmit` clean, `npx vitest run` green.
- `e2e/contactDetailLayout.spec.ts` updated and green at a narrow (≤`md`) viewport, still asserting
  no horizontal overflow and that every section remains reachable in one interaction.
- Hand-verified at mobile (390px), a mid-narrow width just under `md`, and desktop widths: the
  dropdown appears and works below `md`, the existing button row is unchanged at/above it, and
  anchor scrolling still lands each section's title below the sticky nav, not under it.
- All 5 locale files have real translations for any new UI strings.
