# T62 — Chip/badge color system is overloading brand and status colors

| | |
|---|---|
| **Rating** | 3 (visual polish/consistency, not a functional bug) |
| **Size** | M — touches `theme.ts` plus several components; needs a real design decision, not a mechanical recolor |
| **Depends on** | — |
| **Alpha** | n/a — visual-only, no schema/data change |
| **Source** | User's 2026-08-10 mobile/web content-testing notes, plus a live follow-up during triage: "'Clothing size' shows as a tag/chip in Preferences and Gifts for a contact... All preferences have styling like this (such as 'Food' and 'Favorite'). Since we color different types differently based on what they are, this might need an entirely separate color system to avoid overloading the green, info, and warning colors." |

## Why this exists

Two related things came out of the same testing pass:

1. **Category/type badges reuse semantic theme colors that already mean something else.**
   `PreferenceList.tsx` renders every preference's category and key as outlined `Chip`s:
   `color="primary"` (mycelium green — the brand/primary-action color) for the category chip,
   `color="info"` (laccaria purple — normally reserved for informational alerts) for the key chip.
   Confirmed by rendering `category=food, key=favorite` in the dev app: the "Food" chip carries
   `MuiChip-colorPrimary`/`MuiChip-outlinedPrimary` (green border `rgba(158,182,152,0.7)`), "Favorite"
   carries `MuiChip-colorInfo`/`MuiChip-outlinedInfo` (purple border `rgba(159,143,190,0.7)`).
   `ContactHeader.tsx` does the same for Circles (`color="primary"`, filled) and — until this
   session's ad-hoc fix — Tags (`color="secondary"`, filled; now plain default/grey, see landing
   note below). `CircleTagTriagePage.tsx`'s migration wizard uses the identical primary/secondary
   pairing for its circle/tag chips. None of these are actionable buttons — they're read-only
   category labels — but borrowing `primary`/`secondary`/`info` for them means the same green that
   means "this is a clickable brand action" and the purple that means "informational" now also mean
   "this is a food preference," with no third meaning to fall back on if a fourth category needs
   distinguishing later.

2. **Preference category chips ride the same outlined-Chip look everywhere**, so "Food," "Favorite,"
   "Media," and any future category all compete for the same three semantic colors
   (`primary`/`secondary`/`info`) already spoken for by brand actions, tags, and status/alert UI
   respectively — the user's phrase for this was "overloading the green, info, and warning colors."

## What to build

This needs a short design pass before implementation, not a blind recolor — the two live options
identified during triage:

- **Option A (minimal):** drop `color="primary"`/`color="info"` from `PreferenceList.tsx`'s category
  and key chips (and `ContactHeader.tsx`'s Circle chip) the same way this session already did for
  Tags — fall through to MUI's plain default chip (grey fill/border in both themes), matching
  `ConnectionsPanel.tsx`'s undecorated `<Chip size="small" label={...} />` ("N hops" chip), which the
  user named as the reference look. Simple, consistent, but loses any at-a-glance visual
  differentiation between preference categories.
- **Option B (the user's alternate framing):** design a small dedicated categorical palette — e.g.
  2-4 low-saturation tints distinct from `primary`/`secondary`/`success`/`warning`/`error`/`info` —
  for category-style badges specifically (preference category, preference key, and any future
  taxonomy chip), so categories stay visually distinguishable without borrowing colors that already
  carry brand/status meaning elsewhere. Bigger scope: needs new palette entries (see
  `assets/colors/README.md`'s OKLCH derivation process, the same one `theme.ts`'s existing palette
  was built from) verified for contrast in both light and dark mode, plus a decision on how many
  distinct category colors are actually worth having vs. just going neutral.

Whichever direction is chosen, also resolve while in this component:

- **`clothing_size` preferences already had a duplication bug**, fixed ad-hoc during this session
  (see landing note) — they were rendering both in the general Preferences panel's "Other" bucket
  *and* in the dedicated `ClothingSizesPanel` under Gifts. `ContactDetailPage.tsx` now filters
  `PREFERENCE_CLOTHING_SIZE` out of what it passes to `PreferenceList`. No further action needed here
  unless this ticket's redesign touches that call site again.
- **Button affordance** — same triage pass flagged that `outlined` buttons across the app (e.g. "Add
  Preference": `border: 1px solid rgba(158, 182, 152, 0.5)`, transparent background, green text) read
  as fairly subtle/text-like rather than clearly clickable, especially on mobile. Whether that's worth
  changing — a full-opacity border, a light tinted background on hover/idle, or reserving `contained`
  (solid fill) for more section-level "Add X" actions instead of just dialog submit buttons — is the
  same kind of "how much brand-color weight is too much" design call as the chip question above, so
  it belongs in the same design pass rather than a separate uncoordinated change. Audit scope: `grep
  -n "<Button" frontend/src -r` turns up ~200 call sites; most primary-ish actions already use
  `variant="outlined"` (not the bare `text` default), so this is a tuning question about that
  variant's contrast, not a "some buttons have no variant" bug.

## Traps

- Don't just flip every chip/button to `contained`/solid brand green as a reflex fix — that was
  explicitly *not* what either the chip or the button note asked for, and would create a different
  problem (everything reading as an equally-loud primary CTA).
- `MuiChip`/`MuiButton` `styleOverrides` in `theme.ts` apply globally to both light and dark themes
  independently (they're two separate `createTheme` calls) — any palette or opacity change needs
  verifying in both, the same way the existing palette comments document AA/AAA contrast checks.
- If Option B is chosen, new palette entries need the same contrast verification rigor
  `assets/colors/README.md` already applied to `success`/`warning`/`error`/`info` — a new tint that
  fails 4.5:1 with its label color in either mode is a repeat of the laccaria/info correction already
  documented there.

## Done when

- A decision is recorded here (or in `95-backlog-and-priorities.md`) on Option A vs. B before
  implementation starts.
- Preference category/key chips, Circle chips, and any other category-style badge use a consistent,
  deliberately-chosen color treatment — not an incidental reuse of `primary`/`secondary`/`info`.
- Button affordance question is either resolved (a concrete style change) or explicitly closed as
  "current outlined style is fine" with a note why.
- `cd frontend && npx tsc --noEmit && npx vitest run` green.
- Hand-verified in both light and dark mode.

## Landing note

**2026-08-10 (partial, ad-hoc):** Two small, unambiguous pieces of this were fixed directly during
the triage session rather than deferred to this ticket, since they had no real design judgment call
attached:

- `ContactHeader.tsx`'s Tag chips dropped `color="secondary"` (green-tinted fill that read as a
  button) in favor of MUI's plain default chip — the "1 hop" `ConnectionsPanel` look the user asked
  for by name. Circle chips were deliberately left as `color="primary"` (not touched) since circles
  are membership groups, arguably a different case from free-text tags, and folding that question in
  too would have been a guess rather than the user's actual ask.
- `ContactDetailPage.tsx` no longer passes `clothing_size`-category preferences to the general
  `PreferenceList` panel — they already have a dedicated, non-duplicated home in the Gifts tab's
  `ClothingSizesPanel`.

The remaining scope above (the categorical color-system decision, and the button-affordance
question) is still open.

**2026-08-11 (landed):** Decisions recorded and implemented. Scope grew during triage beyond the
original chip/button question to cover the same underlying "where does color mean tappable/brand"
question for icon buttons and hyperlinks — see below.

- **Chips: Option A (neutral).** Dropped `color="primary"/"secondary"/"info"` from every read-only
  category/type/attribute badge: `PreferenceList.tsx`'s category and key chips, `ContactHeader.tsx`'s
  Circle chips (both edit and view mode — no longer a deliberate exception), `CircleTagTriagePage.tsx`'s
  preview chips, `PrepViewPage.tsx`'s "animal" badge, `ReminderList.tsx`'s "flexible" chip,
  `LifeEventList.tsx`'s "remind" flag, `UsersPage.tsx`'s role chip, and `AddContactDialog.tsx`'s
  selected circle/tag chips. Left untouched: `AddNoteDialog.tsx`'s contact-identity chip,
  `NotesPage.tsx`'s unfiled-count badge, and every chip already carrying genuine semantic meaning
  (audit log operation colors, share status, import results, "suggested" relationship badges,
  interactive/filter/autocomplete chips).
- **Buttons: real actions get explicit brand green.** ~17 section/panel-level "Add X" actions switched
  from `variant="outlined"` to `variant="contained" color="primary"` (`ContactDetailPage.tsx`'s six
  `PanelCard` add actions, `ContactsPage.tsx`, `NotesPage.tsx`, `ActivitiesPage.tsx`,
  `HouseholdList.tsx`, `CadencePanel.tsx`, the four Settings "Add" buttons, `GiftList.tsx`,
  `ExternalLinkPanel.tsx`, `AttachmentsSection.tsx`). Deliberately deferred: the small, densely-repeated
  "add another value" buttons inside multi-value field editors and their icon-only `+` adornment
  cousins — solid green on every one of those would overload rather than clarify.
- **Icon buttons: Material Design's interactive/decorative split**, extended from the button decision
  during scoping — decorative field-type glyphs (already `text.secondary`) stay neutral; genuinely
  interactive, previously-colorless utility icons (`CopyButton.tsx`, `EditableField.tsx`'s edit icon,
  `ContactHeader.tsx`'s circle/tag/name edit icons, `ContactInformation.tsx`'s phone-row call/text
  buttons) now get `color="primary"`.
- **Links: fixed the two that rendered browser-default blue.** `GiftList.tsx` and
  `ConversationAgendaList.tsx` converted their raw `<a href>` to MUI `<Link>`, matching
  `ExternalLinkPanel.tsx`'s existing correct pattern — resolves to brand green for free.
  `ContactInformation.tsx`'s deliberate `color: 'inherit'` value-is-the-tappable-target pattern is
  untouched (a different, already-reasoned design).
- **Dark-mode bug found and fixed along the way:** `theme.ts`'s dark `MuiChip` override was
  unconditional (`styleOverrides.root`), silently flattening every semantically-colored chip
  (warning/error/success/info) to plain hypha/bark grey in dark mode only — confirmed live with a
  real overdue reminder's date chip. Rescoped to `variants: [{ props: { color: 'default' }, ... }]`
  so only undecorated chips get the hypha/bark treatment; semantic chips now render their real palette
  colors in dark mode again, matching light mode.
- Verified: `npx tsc --noEmit` and `npx vitest run` (622/622) green. Hand-verified live in the browser
  (real dev-DB contact/reminder/gift data) in both light and dark mode: Add-buttons render solid
  green, copy/edit icons render green, the previously-broken dark-mode chip flattening is fixed, and
  the gift-URL link renders green/underlined instead of browser blue.
- **Bonus fix, folded in:** `RelationshipEdgeDialog.tsx:247` used a plain MUI `Link href=` for an
  internal `/contacts/:id` route instead of composing with react-router's `Link`
  (`component={RouterLink} to=`), causing a full page reload instead of client-side navigation — a
  routing bug, not a color bug, found incidentally during the link audit. Small enough to fix inline
  rather than spin off separately; fixed by importing `Link as RouterLink` from `react-router` and
  switching `href` to `component={RouterLink} to=`, matching the pattern already used elsewhere
  (`AuditPage.tsx`, `DashboardPage.tsx`, `OverdueCadenceList.tsx`). `RelationshipEdgeDialog.test.tsx`
  needed a `MemoryRouter` wrapper added (it previously never rendered a react-router `Link`, so had no
  router context). Hand-verified live: set a `window` marker before clicking the link, confirmed the
  marker survived the navigation (proving client-side routing, not a full reload) and the page landed
  on the correct contact.

**2026-08-11 (Playwright e2e fix).** Giving the Circles/Tags section pencils `color="primary"` (Part C
above) broke `contacts.spec.ts`'s "should edit a contact name" test: it located the profile Save button
via `.locator('.MuiCard-root').first().locator('.MuiIconButton-colorPrimary')` on the assumption Save
was the *only* primary-coloured icon button in the header card — no longer true once the Circles/Tags
pencils (siblings in the same card, always in the DOM regardless of hover state) also carry
`color="primary"`, so the locator now matches 3 elements and Playwright's strict mode refuses to click
it. Fixed by giving the Save/Cancel `IconButton`s real `aria-label`s (`ContactHeader.tsx`, previously
missing — a real a11y gap, not just a test workaround) and switching the test to
`getByRole('button', { name: 'Save' })`. Audited every other spec for the same class of breakage
(button-variant/chip-color/hover-visibility selectors) — this was the only one; everything else already
used text/role/aria-label-based selectors, robust to the color changes. Verified against the real
`docker-compose.test.yml` stack (nginx + backend, same-origin `/api`, matching what CI runs): full
`npx playwright test` — 127/129 pass, the other 2 (`immich.spec.ts`, unrelated to this ticket) pass
cleanly in isolation and are pre-existing parallel-worker flakiness, not a regression.
