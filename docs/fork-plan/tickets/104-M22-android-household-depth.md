# M22 — Household management depth on Android

| | |
|---|---|
| **Rating** | 3 |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | Nothing |
| **Status** | TO BE DONE |

Household core CRUD (create/edit/delete household, add/remove member) already has native parity.
What's missing, per `HouseholdsPage.tsx`/`HouseholdList.tsx`/`AddressHouseholdSuggestions.tsx`:

## Scope

- **Change an existing member's role in place** (`HouseholdsPage.tsx:110-116`,
  `HouseholdList.tsx:248-264`) — `HouseholdDetailViewModel` has `addMember`/`removeMember` only, no
  update-role method; today the only way to change a role on Android is remove-then-re-add.
- **Resolve/display member names**, not raw vCard UIDs — `HouseholdDetailScreen.kt:150-156`
  currently shows `memberVCardUid` as a raw string. Same category of gap as
  [M21](103-M21-android-relationships-depth.md)'s relationships-list finding; consider sharing the
  name-resolution approach between the two tickets if one lands first.
- **Add member: contact search/autocomplete**, not a raw free-text UID field
  (`HouseholdList.tsx:45-116` vs. `HouseholdDetailScreen.kt:176-217`). Same for role — a
  constrained/translated picker from `HOUSEHOLD_ROLES`, not free text.
- **AI/heuristic relationship suggestions within a household** (`HouseholdsPage.tsx:93-107`,
  `HouseholdList.tsx:198-215`) — no equivalent anywhere in Android's households module.
- **T40 shared-address household suggestions**: scan, accept (materialize household), dismiss
  (`HouseholdsPage.tsx:118-165`, `AddressHouseholdSuggestions.tsx`) — no equivalent anywhere on
  Android.

## Done when

- Role can be changed on an existing member without remove-and-re-add.
- Member names resolve and are legible (not raw UIDs) in the household detail screen.
- Adding a member uses contact search, not manual UID entry.
- T40 address-based suggestions are reviewable (accept/dismiss) from Android.
- Hand-verified on-device against an instance with an existing household and at least one pending
  T40 suggestion.
- New strings translated in all five locales.
