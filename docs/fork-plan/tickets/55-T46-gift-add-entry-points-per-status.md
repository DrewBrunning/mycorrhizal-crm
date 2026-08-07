# T46 — Gift "add" entry points default to Idea everywhere; no quick path to Given/Received

| | |
|---|---|
| **Rating** | 3 — same rating as the T20b/T35 gift-tracking work this extends |
| **Size** | S–M |
| **Depends on** | [T35](44-T35-gift-tracking-gaps.md) (done — added the "Add with details" full-form entry point this ticket revises) |
| **Alpha** | n/a — frontend-only wiring change; `GiftInput.status` is already optional and accepted by the API, no backend change needed |
| **Source** | Real-world usage report, 2026-08-06: "Gifts still only prompt to add a gift idea. We need to swap the text/button ('Record a gift idea') to a simple add button that opens the whole form instead or break out the add button to each section so the easy recording mode and the 'Add with details' automatically work for Given/Received as well as Idea" |

## Why this exists

T35 added a second gift entry point ("Add with details") specifically so a gift that was already
given or received could be recorded directly, without creating it as an idea first — but in
practice both entry points still default to Idea:

- The quick-add input's placeholder is literally "Record a gift idea…" (`gifts.placeholder`,
  `en.json:1362`) and its handler, `handleAddGiftItem` (`ContactDetailPage.tsx:536-539`), calls
  `handleCreateGift({ entity_id, description })` with no `status` — `GiftInput.status` is optional
  (`api/gifts.ts:44`), so the record is created with whatever the backend defaults an omitted
  status to (idea).
- "Add with details" (`handleAddFullGift`, `ContactDetailPage.tsx:549-552`) opens `GiftDialog` with
  `gift: null`. `GiftDialog`'s own effect (`GiftDialog.tsx:88`) then does
  `setStatus(gift?.status || 'idea')` — so the full form *also* opens on Idea by default, and the
  user has to remember to change the status dropdown every time they're recording something
  already given or received.

Both entry points live in one row at the top of `GiftList.tsx` (`GiftList.tsx:225-274`), above all
three sections (Ideas/Given/Received) — there's no per-section affordance, so nothing about *where*
you click hints at what status you're recording.

## What to build

Reporter offered two acceptable shapes; recommendation is the second, since it's the one that
actually fixes "no quick path to Given/Received" rather than just relabeling the existing single
entry point:

1. **(Minimal fix, not recommended alone)** Swap the quick-add text input for a plain "+" button
   that always opens the full `GiftDialog` — removes the idea-flavored placeholder text, but still
   leaves every add going through one dialog defaulted to Idea, so it doesn't fully address "give
   me a quick path to Given/Received too."

2. **(Recommended)** Give each section — Ideas, Given, Received — its own pair of entry points
   (quick-add + "add with details"), each pre-seeding the status that section represents:
   - Ideas keeps today's low-friction text-input-and-Enter quick-add (T20b's original point,
     explicitly preserved by T35 — don't remove it), now scoped under the Ideas section header
     instead of floating above all three.
   - Given and Received each get the same quick-add-plus-detail pattern, calling
     `handleCreateGift({ entity_id, description, status: 'given' | 'received' })` for the quick
     path, and opening `GiftDialog` with a new initial-status hint for the detailed path (see
     below) — so recording something already given takes exactly the same number of clicks as
     recording an idea does today, instead of requiring a dialog-open-then-change-dropdown detour.
   - This means `GiftList.tsx`'s three empty-state-suppressed sections (`ideas.length > 0`,
     `given.length > 0`, `received.length > 0`, `GiftList.tsx:287-315`) need to always render their
     header + add row even when empty, so there's still a first entry point into each status
     before any items of that status exist. Check this doesn't reintroduce the T30-class "visible
     header over nothing" bug — the add row itself is the non-empty content justifying the header,
     not a truly empty section.

3. **`GiftDialog` needs an initial-status hint independent of `gift`.** Add e.g. an
   `initialStatus?: GiftStatus` prop, used only when `gift` is null (editing an existing gift
   always keeps using its own status, unchanged). `ContactDetailPage.tsx`'s `handleAddFullGift`
   becomes `handleAddFullGift(status: GiftStatus)`, threading the section's status through to the
   dialog.

## Traps

- Don't change what "given"/"received" *mean* or `handleMarkGivenGift`'s one-click flow
  (`ContactDetailPage.tsx:557+`) — this ticket is only about how a gift enters the system already
  at that status, not the status-transition logic.
- Keep the Ideas quick-add exactly as low-friction as it is today (Enter-to-submit, no dialog) —
  T20b's whole point was opportunistic capture; don't gate it behind an extra click while fixing
  the other two sections.
- `GiftList.tsx`'s current empty state (`items.length === 0`, `GiftList.tsx:281-285`, showing
  `gifts.empty`) covers "no gifts at all" — decide how that interacts with three always-visible
  section add-rows; it may become redundant once every section always shows its own add
  affordance, or it may still be worth keeping as a single first-run hint above all three.
- i18n: new section-scoped placeholder/button text (if the copy changes per section, e.g. "Record
  something given…") needs real translations in all 5 locales, not reused English strings.

## Done when

- `npx tsc --noEmit` clean, `npx vitest run` green.
- Hand-verified: adding a gift as Given (and separately as Received) takes no more steps than
  adding an Idea does today, via both the quick path and "add with details", for both an
  already-empty section and one with existing items.
- `e2e/gifts.spec.ts` updated to cover the new per-section entry points.
- All 5 locale files have real translations for any new/changed strings.
