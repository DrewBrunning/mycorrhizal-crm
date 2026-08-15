# T112 — Android merge search shows contacts that don't contain the typed string

| | |
|---|---|
| **Platform** | Android |
| **Rating** | 4 — wrong results in a picker that feeds a destructive, irreversible merge |
| **Size** | S |
| **Depends on** | Nothing |
| **Status** | **DONE** (2026-08-15) |
| **Source** | Testing note: *"Still seeing extraneous names when searching in a merge action (eg 'Jordan' showing 'Meike')."* |

## Why this exists

Web fixed this exact defect class in [T101](145-T101-merge-picker-strict-match.md): the merge target picker
must client-filter the server's deliberately-broad `GET /contacts?search=` results down to rows whose
displayed name actually contains what was typed, because the endpoint matches name *and* email *and* phone
*and* address *and* FTS tokens. Typing `Jordan` legitimately returns a contact matched only by their address
city or a secondary email — useful for a list, wrong for a picker whose next tap deletes one of two contacts.

M23 shipped the Android merge screen (`MergeContactsScreen.kt`) **without** that filter. Its viewmodel
(`MergeContactsViewModel.onSearchQueryChange`, `:56-78`) calls `contactRepository.listContacts(search = query,
limit = 100)` and stores the whole page, excluding only the keeper:

```kotlin
searchResults = page.contacts.filter { contact -> contact.id != keepId.toInt() },
```

`ContactSearchField.kt` then renders `results` verbatim (`:60-71`) — no client-side predicate. So the Android
merge picker reproduces T101's original report, which is why the note says "still".

## What to build

Filter client-side in `onSearchQueryChange`, mirroring T101's approach. Keep the server query wide (so a
phone/email search still reaches the right contact), but only surface a row when the typed query appears,
case-insensitively, in the name the row displays. `ContactSummary.displayName` (used by
`ContactSearchField` at `:63`) is the string to match against, so the filter and the render can't disagree.

- Add a `contains(query, ignoreCase = true)` predicate over `displayName` to the result set in
  `onSearchQueryChange`. Handle `ContactSummary.displayName` possibly being blank (fall back to not showing
  the row rather than crashing — web's `contactName` falls back to the uid, but a blank displayName with a
  matching uid would still be a name the user can't see, so hiding is the honest choice; document whichever
  is chosen).
- Consider whether to surface a "N shown, M matched on other fields" caption like web's
  `contactMerge.hiddenMatches`. The web ticket required it so silently-hidden rows don't spawn the opposite
  complaint; Android's `ContactSearchField` has no caption slot today, so either add one (new string ×5
  locales) or scope this ticket to filtering-only and note the omission. Recommend the caption for parity.

The `ContactSearchField` is also used by the note/activity form pickers. Those are **not** in scope here —
they are additive pickers, not a destructive merge, and web deliberately left its equivalent (the AppBar
autocomplete) wide. Only the merge screen changes. Do not tighten the shared `ContactSearchField` for
everyone, or you'll regress the note/activity pickers.

## Traps

- The server `limit = 100` truncates before the client filter, so a strict-match contact ranked 101st is
  invisible (same accepted limitation as web T101 — do not make it worse).
- `onSearchQueryChange` clears `searchResults` at the top of each keystroke; the filter must be applied to
  the *fetched* page, not to a stale previous list.
- If a caption is added, it needs a new string in all five locales (per `/CLAUDE.md` frontend/i18n rule, which
  the Android app also follows).

## Done when

- Typing "Jordan" into the Android merge target picker shows only contacts whose displayed name contains
  "jordan" (case-insensitive); a contact matched only by address/email is hidden (or shown with the "matched
  on other fields" count if the caption is implemented).
- Typing a phone number still finds the contact who owns it.
- The note/activity contact pickers are unchanged.
- `./gradlew testDebugUnitTest lintDebug` (the CI gate) green, with a `MergeContactsViewModelTest` case
  feeding a broad server page and asserting the non-matching rows are dropped — hand-verify the test fails
  when the filter is removed.

## Landing note (2026-08-15)

`MergeContactsViewModel.onSearchQueryChange` now filters the server page twice: keeper exclusion first, then
a strict case-insensitive `displayName.contains(query)` pass (web T101's `stringify` equivalent), and stores
the drop count as `hiddenMatchCount` in `MergeUiState`. `ContactSearchField` gained an optional
`hiddenMatchCount: Int = 0` param that renders a "N shown, M matched on other fields" caption
(`merge_hidden_matches`, `%1$d`/`%2$d` placeholders, ×5 locales) only when > 0 — note/activity pickers don't
pass it and are untouched. New `MergeAndBulkViewModelTest` case feeds a broad page ("Meike" matched only on
an unrelated field) and asserts only the name-match survives with a hidden count of 1; hand-verified to fail
(`expected:<[5]> but was:<[5, 7]>`) with the filter removed. One trap hit: the French string needed the
escaped apostrophe (`d\'autres`) or aapt rejected it.

**Review pass (same day)** — fixed a parity gap the first pass introduced: when the strict filter hid *all*
of the server's results, the `results.isEmpty()` branch showed "No contacts found" and suppressed the
hidden-match caption entirely (web shows "0 shown, N matched on other fields"). `ContactSearchField` now
only shows "No contacts found" when `hiddenMatchCount == 0`; when the filter dropped everything the caption
is the only thing rendered. No test change needed — the ViewModel-level case still pins the count, and the
caption-branch fix is a one-line `when` restructuring.


