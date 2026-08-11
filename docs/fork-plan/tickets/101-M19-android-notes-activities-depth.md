# M19 — Notes/Activities depth on Android

| | |
|---|---|
| **Rating** | 4 |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | [M9](91-M9-android-wire-up-existing-screens.md) items 1 (global routes) is independent but related — do that first so there's a natural home for search/filter across all contacts, not just per-contact |
| **Status** | TO BE DONE |

Per-contact `NotesScreen`/`ActivitiesScreen` are real and functional but materially thinner than
web's `NotesPage.tsx`/`ActivitiesPage.tsx`.

## Scope

**Notes** (`NotesScreen.kt:42-108` vs. `NotesPage.tsx`):
- Search notes by text.
- Filter by from/to date.
- Cursor "load more" pagination.
- Delete a note (currently no delete action anywhere in the Android notes list or form).
- Contact-reassignment field on edit (`EditTimelineItemDialog.tsx:155-225` lets you move a note to a
  different contact; `NoteFormScreen.kt:104-145` has no equivalent).

**Activities** (`ActivitiesScreen.kt:41-107` vs. `ActivitiesPage.tsx`):
- Search activities by text.
- Filter by from/to date.
- Cursor "load more" pagination.
- Delete an activity.
- Multi-contact picker on create/edit (`AddActivityDialog.tsx:181-217` is a proper autocomplete
  multi-select; `ActivityFormViewModel.kt:48-56` silently reuses the single route contact instead —
  this means an activity created/edited on Android can never represent more than one participant,
  which is a real behavior gap, not just missing UI polish).
- Contact chips on the activity list card, navigating to each participant.

## Done when

- All items above work per-contact on Android (global-inbox versions are M9's job if not already
  landed).
- An activity edited on Android to add/remove a participant reflects correctly on web.
- Hand-verified on-device: search, date-filter, paginate, delete, and — for activities — edit
  participants and confirm on web.
- New strings translated in all five locales.
