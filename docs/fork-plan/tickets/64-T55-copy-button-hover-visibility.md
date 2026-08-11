# T55 — Copy button should be hidden until hover/tap, matching the edit affordance

| | |
|---|---|
| **Rating** | 2 — cosmetic, but affects every field row in the app |
| **Size** | S |
| **Depends on** | — |
| **Alpha** | n/a — display-only, no schema/API change |
| **Source** | v0.3.0 post-release testing, 2026-08-06 |

## Why this exists — reverses a deliberate T34 decision; read this before implementing

`EditableField.tsx` currently renders the copy button unconditionally whenever a field has a value
(`EditableField.tsx:99`), right next to an edit icon that *is* already hover-gated
(`className="edit-icon"`, `opacity: 0` + the parent's `'&:hover .edit-icon': { opacity: 1 }` at
lines 50-51). This isn't an oversight — there's an explicit comment recording why:

> `/* Every field gets a copy button (T34), tappable or not -- shown whenever there's a value to
> copy, unlike the hover-only edit affordance below. */` (`EditableField.tsx:96-98`)

T34 deliberately kept copy always-visible specifically because hover doesn't exist on touch
devices, and copy was judged more essential to keep discoverable than edit. This ticket asks to
reverse that — hidden until hover **or tap** — which is a real, common mobile pattern (tap a row to
reveal its actions) and isn't the same request T34 rejected (T34's tradeoff was "always visible" vs
"hover-only," not "always visible" vs "hover-or-tap"). But it *is* a deliberate reversal of a
named decision, not a bug fix, so implement it as one: don't just flip the CSS and call it done —
the touch case needs an actual tap-to-reveal interaction, not just an `opacity: 0` copy button that
silently becomes unreachable on a phone.

## What to build

- Apply the same `edit-icon`-style hover-opacity treatment to `CopyButton` usage in
  `EditableField.tsx` (and any other site using the always-visible pattern —
  `ContactHeader.tsx`/`ContactInformation.tsx` per the earlier `CopyButton` usage grep) for pointer
  (hover-capable) devices.
- For touch devices, implement an actual reveal-on-tap interaction (e.g. tapping the field row
  toggles a "revealed" state showing both copy and edit) rather than leaving the button
  hover-gated-and-therefore-unreachable on mobile — a CSS-only `:hover` fix does not satisfy "or
  tap" and would make copy *harder* to use on a phone than it is today.
- Update or remove the T34 comment once the new behavior is decided, so the next person reading this
  file sees the current rationale, not a stale one.

## Traps

- A naive `opacity: 0` + hover-only CSS change satisfies the "hover" half of the request and
  actively regresses the "tap" half — touch devices have no hover state, so copy would become
  invisible-and-unreachable on mobile without deliberate tap-handling. Don't ship that.
- This touches every field row in the app (`EditableField` is the shared component) — a component
  test change here has wide blast radius; check `EditableField.test.tsx` and any snapshot-style
  assertions across contact/field-heavy component tests, not just the one file being edited.

## Done when

- `npx tsc --noEmit` clean, `npx vitest run` green.
- Hand-verified on both a desktop (hover) and a touch/mobile viewport in the browser preview: copy
  is discoverable and usable on both, hidden by default on both.
- The stale T34 comment is updated to describe the new, actual behavior.

## Reverted — 2026-08-11

The hover/tap-to-reveal treatment only worked well for scalar fields; multi-value and non-obvious
tap targets made the copy button unreliable to discover. Removed the reveal state and
`copy-icon`/`&:hover .copy-icon` CSS from `EditableField.tsx` — the copy button is unconditionally
visible again whenever a field has a value, restoring the original T34 behavior. The edit icon's
hover-only treatment is untouched.
