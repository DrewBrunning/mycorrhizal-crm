# T33 — Mobile navigation bar restructuring

| | |
|---|---|
| **Rating** | 4 — hit on every mobile session, not just specific pages |
| **Size** | M |
| **Depends on** | — |
| **Alpha** | n/a — real data exists; this is display-only, no schema change |
| **Source** | v0.2.0-alpha real-world testing |

## Why this exists

The app's top-level navigation bar has accumulated destinations (Contacts, Notes, Households,
Network, Search, Settings, User Management, whatever else has landed since the rebrand) and is
"incredibly crowded" at mobile widths — this is a distinct problem from any individual page's
layout (T28, T32): it's the global chrome every page shares.

## What to build

An actual information-architecture pass on the nav bar, not a mechanical shrink. Concretely:

1. **Inventory every current nav item** and classify each as: primary (used constantly — likely
   Contacts, Search, Notes), secondary (used occasionally — Households, Network, Cadence/agenda
   views if surfaced there), or account-level (Settings, User Management, logout/profile).
2. **Primary items** stay directly visible on mobile, as icons only (no label) once space is
   tight — following the existing convention that icon-only affordances need an accessible label
   even without visible text.
3. **Secondary items** move into a drawer/sidebar (hamburger menu) rather than competing for
   AppBar space directly.
4. **Account-level items** (Settings, User Management, profile) collapse into the existing
   account/profile menu if one exists, rather than sitting in the main nav row.
5. Decide the breakpoint(s) at which this collapsing happens — likely the same `sm` (600px)
   breakpoint T28 established for the contact page, for consistency across the app rather than a
   nav-specific one-off.

## Traps

- This is a design decision, not just implementation — the classification in step 1 should be
  confirmed against actual usage patterns rather than guessed, since getting a "primary" item
  wrong means the user has to open a drawer for something they reach for constantly.
- Whatever pattern is chosen (drawer, bottom nav, overflow menu) should be consistent with any
  existing mobile chrome pattern already in the app (check if `AppLayout`/`AppBar`-equivalent
  already has a drawer for anything) rather than introducing a second navigation paradigm.
- Icon-only nav items need `aria-label`s — don't drop accessible names when dropping visible
  labels.

## Done when

- At 360–390px width, the nav bar does not visually crowd/overflow/wrap unpredictably.
- Every previously-reachable destination is still reachable in at most one extra tap (opening a
  drawer/menu counts as one tap).
- `npx tsc --noEmit` clean, `npx vitest run` green.
- Visually verified at 360px, 390px, and 414px, and that a screen reader / accessibility check
  (`aria-label`s present) passes for icon-only items.

**Done, 2026-08-05.** The phone-width AppBar now shows primary destinations (Contacts, Search,
Notes) as icon-only buttons, each with an aria-label; secondary items live in the hamburger
drawer; and account-level items (Settings, Data settings, User Management, logout) collapse
into a new account menu. The collapse happens at the `sm` breakpoint, matching the 600px
convention T28 established for the contact page, so sm-md tablets and desktop are unchanged.
`app.accountMenu` translated in all five locales. New `e2e/navMobile.spec.ts` pins the
icon-only primary row, direct navigation, one-tap drawer reachability, the account menu, and
no AppBar-induced horizontal overflow at 360/390px.
