# T80 — Address editor has no line 2 / PO box / floor field (web)

| | |
|---|---|
| **Platform** | Web |
| **Rating** | 3 — the user-visible half of T79 |
| **Size** | S — three fields on one component |
| **Depends on** | [T79](123-T79-flat-address-projection-too-narrow.md) — the fields don't exist on the model yet. Blocked until it lands. |
| **Status** | TO BE DONE |
| **Source** | User question during the 2026-08-11 grooming pass: *"I don't see an address line 2 or anything in the web UI, so an address like '123 Fake St, Apt 456, Townesville, MO 55555' currently needs everything together on the one address line."* |

## Why this exists

`AddressFields.tsx` renders exactly five inputs — street, city, region, postal, country — matching
the five fields the flat `ContactAddress` had before
[T79](123-T79-flat-address-projection-too-narrow.md). So an apartment or PO box has nowhere to go
except crammed into the street line.

That is not data loss (nothing parses or rewrites the street string — see T79's analysis), but it
does mean the app can't represent an apartment as its own thing, and it means a **VCF-imported**
address whose apartment *was* parsed into a real component has no editor that can show or change it.

## What to build

1. Add PO box, apartment (address line 2), and floor inputs to `AddressFields.tsx`, wired to the
   fields T79 adds to `ContactAddress`. Include them in the component's `defaultAddress` initializer
   (`AddressFields.tsx:26-30`) so a newly added address starts with the keys present.
2. **Progressive disclosure — decided 2026-08-11.** Eight always-visible inputs per address, times
   multiple addresses, is too much vertical weight for fields most contacts leave empty. So:

   - The five existing fields (street, city, region, postal, country) stay always visible.
   - Apartment, PO box, and floor are **hidden by default**, behind a reveal control on each address
     block.
   - **They auto-expand when any of them is non-empty.** This is the part that must not be missed:
     a VCF-imported address carrying a PO box has to show it without the user first guessing that a
     hidden section exists. Hidden-by-default applies to the *empty* case only.
   - Once revealed, the section stays open for that address for the rest of the editing session.

   **Label it "Additional fields", not "Add more."** The address block already sits next to an
   add-another-address control, and "Add more" there reads as "add another address." Whatever the
   final wording, it must not collide with that.

   The reveal is per-address, not per-form — two addresses can be in different states, and a shared
   toggle would force the sparse one to show empty inputs alongside the detailed one.
3. Check the read-only address rendering too, not just the editor — `ContactInformation.tsx`'s
   address `render*List` and anywhere `FormatAddress`'s output is displayed should show the new parts.
4. New strings translated in all five locale files (`en`, `de`, `es`, `fr`, `it`) — `/CLAUDE.md`
   frontend trap #5; `src/i18n/locales.test.ts` enforces it.

## Traps

- **Coordinate with [T74](118-T74-desktop-field-row-action-distance.md)** if it lands first — that
  ticket puts field rows into a two-column grid at `lg`+, so an address editor that grew three inputs
  is rendering in a ~530px column, not a ~1136px one. Either ticket landing first is fine; just don't
  design the field layout against the wrong width.
- **`getByLabelText` needs the MUI `" *"` suffix** for required fields in tests (`/CLAUDE.md` frontend
  trap #2), and new component test files need an explicit `afterEach(cleanup)` (trap #1).

## Done when

- A contact's address can be entered as street + apartment/PO box/floor + city/region/postal/country,
  and round-trips through save and reload.
- An address with none of the extra parts shows the same five fields it does today, plus one
  unobtrusive reveal control.
- **A VCF-imported address carrying a PO box or extended-address part shows those parts expanded on
  load**, with no extra tap — the auto-expand rule, and the case most likely to be got wrong.
- With two addresses on one contact, revealing the extra fields on one does not expand the other.
- New strings translated in all five locales.
- `cd frontend && npx tsc --noEmit && npx vitest run` green.
