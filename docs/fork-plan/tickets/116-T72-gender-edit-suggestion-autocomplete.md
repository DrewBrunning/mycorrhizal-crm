# T72 — Gender edit is a bare text field, not the suggestion-autocomplete Add used to have

| | |
|---|---|
| **Platform** | Web |
| **Rating** | 3 — UX consistency/polish, self-contained |
| **Size** | S — one shared component + one call site |
| **Depends on** | Nothing |
| **Status** | TO BE DONE |
| **Source** | Testing notes, 2026-08-11: "Gender editing on a Contact doesn't have the defaults pop up the way it does on Contact entry (add)" |

## Why this exists

Gender's edit widget has never had suggestion defaults, on any build. `ContactInformation.tsx:742-757`
renders it through the generic `EditableField` component with only `value`/`formattedDisplayValue` —
`EditableField.tsx:70-80` is unconditionally a plain MUI `TextField`, with no `options`/`Autocomplete`
capability at all. It's shared by many unrelated scalar-text fields (birthday, organization,
how-we-met, job title, ...), none of which need a suggestion list, so it never grew one.

The behavior the report remembers from Add did exist, historically: pre-T52,
`AddContactDialog.tsx` rendered gender as an MUI `Autocomplete` with `freeSolo` **and**
`options={[...GENDER_OPTIONS]}` (`male`, `female`, `non_binary`, `genderfluid`, `other`,
`prefer_not_to_say`, from `frontend/src/contactFields.ts:133`) — defaults popped up on focus while
still accepting free text. [T52](61-T52-simplify-contact-add-flow.md) ("Simplify the contact-add flow
to name + contact fields") removed gender from Add entirely, along with every other non-essential
field — that's a deliberate, still-current design decision on `main`, not something this ticket
reopens.

`contactFields.ts:128-132`'s own comment confirms the intended shape: *"Gender is a legacy free-text
CRM field... these are convenience suggestions for the free-solo Autocomplete, not a constrained
set."* — i.e., the codebase already documents that Edit *should* be a free-solo Autocomplete with
`GENDER_OPTIONS`, matching the pattern `SpeakToAsEditor.tsx:115-128` uses for the related grammatical-
gender field (there a true `TextField select` + `MenuItem`, since GRAMGENDER is a real RFC 9554 enum
rather than free text — gender stays free-solo).

**Scope, confirmed with the user 2026-08-11**: fix Edit only. Do not restore gender to the Add
dialog — that would revisit T52's still-current decision and is out of scope here.

## What to build

1. Give `EditableField` (or a small `gender`-specific wrapper around it) an optional `options: string[]`
   prop; when set, render an MUI `Autocomplete freeSolo` in edit mode instead of the plain `TextField`
   (`EditableField.tsx:67-93`).
2. Wire `options={[...GENDER_OPTIONS]}` and the same `getOptionLabel` i18n mapping the old Add-dialog
   code used, at the one call site (`ContactInformation.tsx:742-757`).
3. No backend/API changes — gender is already free text server-side; this is purely an edit-widget
   change.

## Done when

- Editing a contact's gender field shows the same suggestion dropdown (male/female/non_binary/
  genderfluid/other/prefer_not_to_say, localized) on focus, while still accepting free text.
- `EditableField`'s existing consumers for other fields (birthday, organization, etc.) are unaffected
  — the new `options` prop is opt-in.
- Hand-verified in the browser: focus the gender field in edit mode, confirm the dropdown appears,
  confirm free text still saves.
