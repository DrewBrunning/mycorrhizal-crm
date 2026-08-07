# T54 — Contact header's actions menu shifts position when the name wraps

| | |
|---|---|
| **Rating** | 2 — small, cosmetic, but a real annoyance on every long-named contact |
| **Size** | S |
| **Depends on** | — |
| **Alpha** | n/a — display-only, no schema/API change |
| **Source** | v0.3.0 post-release testing, 2026-08-06 |

## Why this exists

The name and the actions menu are laid out as flex-wrap siblings
(`ContactHeader.tsx:303-314`: `display: 'flex', justifyContent: 'space-between', flexWrap: 'wrap'`)
inside the same row, with the name (`ContactHeader.tsx:315-331`) and the `MoreVertIcon` menu button
(`ContactHeader.tsx:332` on) both participating in normal flex flow. When a long enough name wraps
to a second line, the wrapping shifts where the menu button ends up, since it's a sibling in the
same wrapping row rather than pinned independently to the corner.

## What to build

- Take the actions-menu trigger (and the profile-edit icon it's grouped with) out of the wrapping
  flex flow that the name participates in — position it independently (e.g. absolute/fixed relative
  to the header card, matching the pattern the header's own profile-photo edit overlay already uses
  at `ContactHeader.tsx:141-182` with `position: 'relative'`/`'absolute'`) so it stays pinned to the
  upper-right corner regardless of how many lines the name wraps to.
- Verify at both narrow (mobile) and wide viewports, and with a name long enough to wrap to two and
  three lines.

## Traps

- Don't just increase the name's max-width/truncate it to avoid wrapping — long real names (this app
  already handles `overflowWrap: 'anywhere'` at `ContactHeader.tsx:316`) are expected to wrap; the
  fix is to stop the menu from moving when they do, not to stop them from wrapping.
- Keep the edit-profile icon button (`ContactHeader.tsx:319-330`) working the same way it does today
  — this ticket is about the actions menu's position, not a broader header layout rewrite.

## Done when

- `npx tsc --noEmit` clean, `npx vitest run` green (updated `ContactHeader` component test asserting
  position stability, if the existing tests don't already cover layout).
- Hand-verified in the browser preview: a contact with a long enough name to wrap to 2-3 lines keeps
  the actions menu in a fixed upper-right position, at both desktop and mobile viewport widths.
