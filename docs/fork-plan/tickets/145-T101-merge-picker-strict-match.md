# T101 — The merge contact picker returns contacts that don't contain what you typed

| | |
|---|---|
| **Platform** | Web |
| **Rating** | 4 — wrong results in a picker that feeds a destructive, irreversible action |
| **Size** | S |
| **Depends on** | Nothing |
| **Status** | **DONE** (2026-08-14) |
| **Source** | Beta testing note, 2026-08-13: *"Contact search in merge needs to be strict string match — it's matching contacts who don't contain the typed string."* |

## Why this exists

`MergeContactsDialog` fetches with `getContacts({ limit: 100, search })`
(`frontend/src/components/MergeContactsDialog.tsx:60-73`, debounced 300ms at `:80-85`) and then renders
whatever comes back **verbatim** — the MUI `Autocomplete` is given `filterOptions={(x) => x}` at `:138`,
which disables client-side filtering entirely.

That endpoint is `GET /contacts?search=` → `applyContactSearch`
(`backend/controllers/contact_controller.go:105-160`), which is deliberately broad because three other
consumers need it to be:

- The LIKE clause spans `firstname`, `lastname`, `nickname`, the joined name forms, `email`, `phone`,
  **`addresses_flat`** (`:117`) and a `json_each(contacts.emails)` scan (`:118`) — so a match on a city or
  a secondary email surfaces a contact whose name contains nothing you typed.
- Phone handling adds `phones_normalized LIKE ?` (`:132`) plus, for phone-shaped terms, digit-string and
  `PhoneKey` comparisons (`:134-140`). `PhoneKey` truncates to the last 10 digits
  (`backend/models/phonekey.go:15-24`), so country-code and punctuation variants match.
- [T85](129-T85-contacts-list-fts-search.md) **ORs an FTS5 token-prefix match** on top (`:148-157`), gated
  at two runes, via `services.ContactFTSMatch` (`backend/services/search_service.go:150-155`). Typing `ann`
  returns everyone with any indexed token starting `ann`.

The union of those is right for a contacts list, where a wide net is a feature. It is wrong for a picker
whose next click permanently deletes one of the two contacts.

## What to build

**Filter client-side. Do not narrow `applyContactSearch`.** That function is shared by the Contacts list,
the AppBar autocomplete and the merge picker; narrowing it would regress
[T85](129-T85-contacts-list-fts-search.md), [T86](130-T86-web-fold-search-into-contacts.md) and
[T69](113-T69-phone-search-tokenization.md), all of which depend on the current breadth.

Replace `filterOptions={(x) => x}` at `MergeContactsDialog.tsx:138` with a predicate that keeps an option
only when the typed string appears, case-insensitively, in the text the option actually renders — the same
display string `getOptionLabel` produces. MUI's `createFilterOptions({ stringify: getOptionLabel })` gives
exactly this; use it rather than hand-rolling.

Keep the server fetch as-is. The wide query is what makes the right contact *reachable* (searching a phone
number still finds them); the client filter is what stops unexplained rows appearing once you've typed a
name.

**Show a count when the server returned more than the filter kept** — e.g. "3 shown, 12 matched on other
fields" — with no way to expand it in this ticket. Silently hiding server results in a picker is how the
opposite complaint gets filed later.

## Traps

- `limit: 100` (`:61`) means the server truncates before the client filters, so a strict-match contact
  ranked 101st is invisible. That is pre-existing and acceptable at typical address-book sizes, but do not
  make it worse by filtering a smaller page.
- The AppBar autocomplete (`frontend/src/App.tsx`) hits the same endpoint and is deliberately *not* changed
  — jump-to-contact benefits from the wide net. Only the merge picker gets the strict filter.
- New component test files need an explicit `afterEach(cleanup)` (`/CLAUDE.md` frontend trap #1).

## Done when

- Typing a name into the merge picker shows only contacts whose displayed label contains that string,
  case-insensitively.
- Typing a phone number still finds the contact who owns it (the server query is unchanged).
- When the server returns rows the filter drops, the count is shown rather than silently discarded.
- The AppBar contact autocomplete's behaviour is unchanged.
- New strings translated in all five locales.
- `cd frontend && npx tsc --noEmit && npx vitest run` green, plus a Playwright spec typing a partial name
  and asserting no non-matching row is offered.

## Landing note (2026-08-14)

Implemented as scoped: `MergeContactsDialog.tsx`'s single-mode `Autocomplete` now uses
`createFilterOptions({ stringify: contactOptionLabel })` in place of the pass-through `filterOptions={(x)
=> x}`. `contactOptionLabel` was hoisted to a module-level constant shared between `getOptionLabel` and
the filter's `stringify` so both agree on what "the text the option actually renders" means. A
`hiddenMatchCount` (`contacts.length - filteredContacts.length`, computed with the same filter fn against
the current `searchInput`) drives a caption below the field — `contactMerge.hiddenMatches`, "{{shown}}
shown, {{hidden}} matched on other fields" — shown only when the filter actually hid something. Server
query, `limit: 100`, and the AppBar autocomplete are all untouched, per the ticket.

Added four component tests (a server result matched only via a non-name field is hidden; matching is
case-insensitive; the hidden-count caption appears/doesn't as appropriate) — hand-verified by reverting
`filterOptions` to the old pass-through and confirming the two matching-behavior tests fail. One trap hit
along the way, worth knowing for future Autocomplete tests in this file: MUI's `useAutocomplete` resets
its internal input value on every render while the input isn't focused, so a bare `fireEvent.change`
(no prior `fireEvent.focus`) makes jsdom's Autocomplete fall back to showing every option unfiltered even
though the surrounding component's own state updated correctly — a real browser always fires focus before
input, so this only bites tests. Fixed by adding `fireEvent.focus(input)` before `fireEvent.change`
everywhere in this test file, including the pre-existing `selectBob` helper (harmless there since it only
ever had one option to show).

Also verified end-to-end against a real backend + browser (not just mocked tests): created a contact
matched only by an address field ("Carson City" for typed "car"), opened the merge picker from a third
contact, and confirmed the dropdown showed only the name-matching contact with "1 shown, 1 matched on
other fields" — the exact scenario from the ticket's source beta report. Confirmed selecting the shown
result still loads a normal merge preview, and confirmed the AppBar search dropdown (out of scope) was
unaffected — it turns out the AppBar already effectively strict-filters via MUI's untouched default
`filterOptions`, since it never set the `(x) => x` override this ticket removes from the merge picker;
its "wide net" advantage lives in the full search-results page reached via Enter, not in its dropdown.
