# T47 — Field action icons (call/text/copy) should sit near the edit button, not crowd the value text; phone number itself should be a tel: link too

| | |
|---|---|
| **Rating** | 3 — cosmetic/layout, but touches every field on the most-viewed card of the most-used page |
| **Size** | M |
| **Depends on** | [T34](43-T34-contact-field-linking.md) (done — owns every render function and the phone tel:/sms: decision this revises) |
| **Alpha** | n/a — frontend-only, no schema/API change |
| **Source** | Real-world usage report, 2026-08-06: "The icons like call, text, copy, etc should be over by the edit button, not up against the content. Additionally, I thought we decided to make the phone number directly a tel: link when available (the button is also nice, let's keep it in addition to making the phone number a link itself)" |

## Why this exists

### 1. Action icons crowd the value text instead of grouping toward the edit button

`ContactInformation.tsx`'s field rows are built from `EditableArrayField` (`EditableArrayField.tsx`):
icon, then a flexible value column, then a right-aligned edit pencil that fades in on hover
(`className: 'edit-button'`, `EditableArrayField.tsx:88-96`). Inside that value column, every
`render*List` function wraps each row's value and its action icon(s) in the *same* tight flex box:

```tsx
<Box sx={{ display: 'flex', alignItems: 'center', gap: 0.25 }}>
  <Typography variant="body2">{r.value}</Typography>
  <CopyButton value={r.value} label={...} />
</Box>
```

— which puts the copy button (and, for phone, the call/text buttons) immediately butted up against
the text with only a 2px gap, rather than aligned toward the row's right edge near the edit button.
This pattern repeats identically across every field with an action icon:

- `renderPhoneList` — `ContactInformation.tsx:198-231` (call + text + copy)
- `renderEmailList` — `ContactInformation.tsx:233-248` (copy)
- `renderUriValueList` (Card.Links, IMPP) — `ContactInformation.tsx:254-286` (copy)
- `renderAddressList` — `ContactInformation.tsx:288-311` (copy)
- `renderOnlineServices` — `ContactInformation.tsx:313-337` (copy)
- `renderPersonalInfo` — `ContactInformation.tsx:339-355` (copy)
- `renderCardNotes` — `ContactInformation.tsx:371-383` (copy)
- `renderPreferredLanguages` — `ContactInformation.tsx:385-400` (copy)
- `renderSpeakToAs` — `ContactInformation.tsx:402-418` (copy)
- `renderAnniversaries` — `ContactInformation.tsx:420-438` (copy)
- (`renderKeywords`, `ContactInformation.tsx:357-369`, uses a row-direction wrap instead — same
  crowding, different flex direction)

### 2. Phone number itself isn't a tel: link

T34's landing note made a deliberate call: "Single-action fields... became the tappable value
itself... phone — the only multi-action field — gets discrete call/text icon buttons instead,"
specifically because wrapping the *whole* phone value in one link was ambiguous between call and
text. That reasoning holds for *replacing* the buttons with a single link, but doesn't rule out
doing *both*: the number is still unambiguous as a tel: link (texting isn't a browser default
action on a plain link the way calling is), so making the value itself tappable via `buildTelLink`
— already imported and used for the call button's `href` (`ContactInformation.tsx:213`) — while
keeping the discrete call/text/copy buttons, gives the fastest, most obvious action (call) two
ways to trigger it without removing the only way to text.

## What to build

### Layout: pull action icons toward the row's right edge

Introduce one shared row-layout piece — a small component or a shared `sx` — used by all the
`render*List` functions above, replacing the ad hoc `Box sx={{ display: 'flex', alignItems:
'center', gap: 0.25 }}` wrapper with one that pushes the value left and the action-icon cluster
right within the row (e.g. `justifyContent: 'space-between'`, value in a `minWidth: 0,
overflowWrap: 'anywhere'` box, actions in a `flexShrink: 0` box), so each row's icons line up
toward the same right edge the field's own edit pencil sits at, rather than trailing immediately
after the text. This is the kind of duplicated pattern the `simplify` skill is built for — factor
it once rather than editing eleven near-identical call sites independently by hand.

Keep multi-value rows (a contact with three phone numbers, say) each getting their own row-level
alignment — the ask is per-row spacing, not a single actions cluster shared across multiple stacked
values.

### Phone: link the value, keep the buttons

In `renderPhoneList` (`ContactInformation.tsx:198-231`), wrap the phone `Typography` itself as
`component="a" href={buildTelLink(r.value)}` (matching the styling `renderEmailList`'s `mailto:`
link and `renderUriValueList`'s tappable case already use — `sx={{ color: 'inherit' }}`), and keep
the existing call/text/copy `IconButton`s exactly as they are today, just repositioned per the
layout change above.

## Traps

- **Don't regress T34's copy-button-on-every-field guarantee** while refactoring the shared
  wrapper — every field still needs its action icon(s), just positioned differently.
- **Fax numbers stay non-tappable** (`isFax` check, `ContactInformation.tsx:203, 211`) — the new
  `<a href>` wrapper on the value must respect the same `!isFax` gate the call button already uses,
  not just skip the call *button* while still linking fax text.
- **`aria-label`s stay meaningful** once the value itself is also a link — a screen reader now has
  two ways to reach "call this number" (the linked text and the discrete button); make sure neither
  becomes ambiguous or redundant to the point of confusion (e.g. the linked text doesn't need its
  own new `aria-label` beyond the visible number, since that's standard link behavior, but confirm
  with a real screen reader pass or at minimum the existing a11y-lint step in CI).
- **`overflowWrap`/`wordBreak` behavior** varies by field today (`renderOnlineServices` and
  `renderUriValueList` use `wordBreak: 'break-all'`, others don't) — preserve each field's existing
  wrapping behavior when moving them onto the shared layout, don't flatten them to one style as a
  side effect.
- This is the same repo-wide pattern [T44](53-T44-link-field-type-registry-not-in-editors.md) and
  [T43](52-T43-link-field-type-custom-icons.md) also touch (`ContactInformation.tsx`'s render
  functions, `linkResolution.ts`) — no hard dependency, but coordinate branch order to avoid a
  three-way merge conflict in the same file if working on more than one of these at once.

## Done when

- `npx tsc --noEmit` clean, `npx vitest run` green, including a layout-level test (or a visual/DOM
  assertion) that action icons render in a right-aligned cluster distinct from the value text, for
  at least phone and one other field type.
- A test proves a non-fax phone number's own text is now a `tel:` link (via `buildTelLink`) while a
  fax number's is not, and that the call/text/copy buttons are unchanged and still present
  alongside it.
- Hand-verified in a real browser at desktop and mobile widths: tapping the phone number itself
  dials, the call/text/copy buttons still work independently, and every other field's action
  icon(s) visually sit toward the row's right side rather than jammed against the text.
- No regression in existing field-wrapping/`overflowWrap` behavior per field.
