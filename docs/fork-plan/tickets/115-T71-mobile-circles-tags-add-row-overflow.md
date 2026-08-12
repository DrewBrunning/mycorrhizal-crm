# T71 — Mobile web: circles/tags "add" row overflows the screen, blocking use

| | |
|---|---|
| **Platform** | Web (mobile viewport) |
| **Rating** | 4 — blocks a core interaction (adding a circle/tag) entirely on mobile web |
| **Size** | S — CSS/layout only, one file |
| **Depends on** | Nothing |
| **Status** | **DONE**, 2026-08-12. Both edit-mode rows in `ContactHeader.tsx` (circles at 582, tags at 643) now get `flexWrap="wrap"`, and the `Autocomplete`'s hard `minWidth: 200` floor becomes `{ xs: '100%', sm: 200 }` so it takes a full line at phone widths instead of imposing a fixed floor alongside its siblings; the `TextField` gained `minWidth: 0` so it can shrink below its content width once wrapped. Hand-verified at a 390px viewport: pre-fix, the Add button's left edge sat at x=394 (off the 390px viewport, unreachable); post-fix, both rows wrap onto their own lines and the Add button lands at x=158–222, fully on-screen. Desktop (`sm`+) is pixel-identical to before — confirmed the `Autocomplete` still renders at its 200px floor on one line. Landed as part of the T71/T72/T78 web rollup branch. |
| **Source** | Testing notes, 2026-08-11: "Mobile web Circles input on a Contact overflows the screen and prevents use" |

## Why this exists

`ContactHeader.tsx` renders the circle/tag "add" controls as a `Stack direction="row"` with no wrap
and no responsive `direction` override — MUI `Stack` in row mode never wraps children onto new lines
by default:

```tsx
// frontend/src/components/ContactHeader.tsx:582-604 (circles), duplicated at 643-664 (tags)
<Stack direction="row" spacing={1} sx={{ mt: 1 }}>
  <Autocomplete size="small" ... sx={{ minWidth: 200 }}
    renderInput={(params) => <TextField {...params} label={t('contacts.selectCircle')} size="small" />} />
  <TextField size="small" placeholder={t('contactDetail.newCircle')} ... sx={{ flexGrow: 1 }} />
  <Button size="small" variant="contained" disabled={!newCircleName.trim()}>{t('contactDetail.add')}</Button>
</Stack>
```

Three things combine to overflow the row on a phone-width viewport:
1. No `flexWrap`/responsive `direction` on the `Stack`.
2. `Autocomplete sx={{ minWidth: 200 }}` — a hard floor for just one of three children.
3. `TextField sx={{ flexGrow: 1 }}` plus an unconstrained `Button` — `flexGrow` only redistributes
   *extra* space, it can't shrink the row below its children's combined minimum widths.

On a 360–390px phone, `Autocomplete (≥200px) + TextField's min content width + Add button` exceeds
the viewport, and with no `overflow-x: hidden` upstream, the whole page gains horizontal
scroll/overflow rather than the row wrapping — pushing part of the row (often the Add button or the
new-name field) off-screen and out of reach.

**Note the chip *display* rows are fine** — the sibling rows at lines 572/606/633/667 already use
`Stack direction="row" flexWrap="wrap" sx={{ gap: 1 }}`, so existing circle/tag chips already wrap
correctly. It's specifically the two edit-mode "add" rows (circles at 582, tags at 643) that lack it.

**There's an in-repo precedent for the fix.** `ContactHeader.tsx:107-109` already has a
`compactActions = useMediaQuery(theme.breakpoints.down('md'))` flag (added per T28) that collapses
the header's action buttons on narrow viewports — never extended to these two rows. Ticket
[T28](21-T28-mobile-contact-layout.md) documents the same overflow class and its accepted fixes
(aggressive `sm` reflow, action collapsing). `ContactsPage.tsx:171`'s circle-filter bar shows the
correct comparison pattern for a similar circle-related control: `Stack
direction={{ xs: 'column', sm: 'row' }}` plus `flexWrap`, so it can never produce a too-wide row on
mobile — the ContactHeader edit rows just never got the same treatment.

## What to build

In `ContactHeader.tsx`, for both the circles "add" row (~582-604) and the tags "add" row
(~643-664):
1. Add `flexWrap="wrap"` (or a responsive `direction={{ xs: 'column', sm: 'row' }}`), matching the
   pattern already used at the sibling chip-display rows and in `ContactsPage.tsx:171`.
2. Drop or reduce the `Autocomplete`'s `minWidth: 200`, or give it `fullWidth` when stacked at `xs`,
   so it doesn't impose a floor wider than a 360px viewport allows alongside its siblings.

No architectural change needed — `ContactHeader.tsx` is the one component used for both viewing and
editing circles/tags; this is confined to the two edit-mode rows' layout props.

## Done when

- On a 360-390px-wide viewport, adding a circle and adding a tag from a contact's page both work
  without any part of the control row being pushed off-screen or requiring horizontal scroll.
- Hand-verified in the browser preview at a mobile viewport width (not just desktop-resized).
- Existing chip-display wrapping and desktop layout are unaffected.
