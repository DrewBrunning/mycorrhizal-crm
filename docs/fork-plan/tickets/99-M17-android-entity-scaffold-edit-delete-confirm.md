# M17 — Android entity-list scaffold: add edit + delete-confirmation

| | |
|---|---|
| **Rating** | 4 — one fix resolves four entity types at once |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 — structural finding #3 |
| **Depends on** | Nothing |
| **Status** | TO BE DONE |

Life Events, Gifts, Preferences, and Conversation Agenda all share one generic Android scaffold —
`EntityListScreens.kt` (`android/feature/timelineentities/`) backed by one generic
`EntityListUiState`/`EntityItem` shape in `TimelineEntitiesViewModel.kt:39-48`. That shared scaffold
structurally has **no edit affordance** and **no delete-confirmation dialog**: `EntityListScreens.kt`
wires create (FAB → dialog) and delete (immediate, no confirm, `:140-142`) only. Every one of the
four entities inherits both gaps identically. Fixing the scaffold once is far cheaper than four
separate per-entity tickets for the same two problems — this ticket is scoped to exactly that,
before [M18](100-M18-android-entity-field-richness.md) adds the per-entity field depth each of
these four still lacks even after this lands.

## Scope

1. Add a delete-confirmation `AlertDialog` to the shared scaffold (web confirms via
   `window.confirm` on all four — `LifeEventList.tsx:90-92`, `GiftList.tsx:208-212`,
   `PreferenceList.tsx:74-81`, `ConversationAgendaList.tsx:68-72,90-97`). One implementation, reused
   by all four call sites.
2. Add a tap-to-edit path to the shared scaffold (currently rows have no click handler at all in
   three of the four; only gifts render an "open URL" icon action, which is unrelated). This can
   reuse each entity's existing create-dialog component in edit mode, the same pattern web's
   `EditTimelineItemDialog.tsx` uses relative to its add-dialogs.

## Done when

- Deleting any of the four entity types on Android requires confirmation, matching web.
- Editing any of the four entity types is reachable by tapping the row, at minimum for the fields
  the current create dialog already models (field-level richness is M18's job, not this ticket's).
- A single shared-scaffold change accounts for the fix landing on all four entities — not four
  separate patches.
- Hand-verified on-device for each of the four entity types: create, edit, attempt delete-and-cancel,
  delete-and-confirm.
