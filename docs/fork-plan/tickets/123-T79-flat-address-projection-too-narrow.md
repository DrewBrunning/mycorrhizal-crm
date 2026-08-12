# T79 — The flat address projection has no slot for apartment / PO box / floor (backend)

| | |
|---|---|
| **Platform** | Backend |
| **Rating** | 3 — no loss for hand-typed addresses, real invisibility for imported ones |
| **Size** | S–M — a JSON-shape widening plus four mapping functions |
| **Depends on** | Nothing, but see the ordering note under Traps re: [T75](119-T75-plain-save-destroys-card-only-data.md). Blocks [T80](124-T80-web-address-editor-line-two.md). |
| **Status** | TO BE DONE |
| **Source** | User question during the 2026-08-11 grooming pass: *"do we support postOfficeBox, apartment, and floor? I don't see an address line 2 in the web UI."* |

## Why this exists

Three layers disagree about how much of an address this project can represent.

**The neutral model and the vCard adapters support the full set.**
`contactmodel.AddressComponent.Kind` covers the 17 RFC 9553 kinds — `room`, `apartment`, `floor`,
`building`, `number`, `name`, `block`, `subdistrict`, `district`, `locality`, `region`, `postcode`,
`country`, `direction`, `landmark`, `postOfficeBox`, `separator` (`contactmodel/model.go:167-176`).
`vcard4/adapter.go:944,961-965` maps `postOfficeBox`/`apartment`/`floor`/`number`/`block` in both
directions, and `vcard3/adapter.go:443,688` maps `postOfficeBox`.

**The flat `ContactAddress` supports five.** `models/contact.go:61-68` is
`Type/Street/City/Region/Postal/Country` — no sub-street slot at all. Four functions encode that
narrowing:

- `contactAddressFromNeutral` (`contact_record_reverse.go:283-303`) — nested → flat, switches on the
  five kinds it can store.
- `AddressFromContactAddress` (`contact_record.go:618-630`) — flat → nested, emits exactly
  `name`/`locality`/`region`/`postcode`/`country`.
- `FormatAddress` (`contact.go:183-191`) and `FlattenAddresses` (`:199`) — the display scalar and the
  T38 search column, both hardcoding the same five.

**The web editor supports those same five** (`AddressFields.tsx` renders street/city/region/postal/
country), which is the "no address line 2" the question observed.

### What this actually costs

**Hand-typed addresses lose nothing.** There is *no* parsing anywhere — `AddressFromContactAddress`
maps the `street` field to a single `name` component verbatim, with no comma-splitting or heuristics.
Typing `123 Fake St, Apt 456` leaves that whole string as the street-name component, and it exports
into vCard's street field intact. It is unstructured, not lost: nothing can filter or sort on the
apartment, and a strict downstream consumer reads the apartment as part of the street name.

**Imported addresses do lose visibility.** vCard `ADR` is seven parts — PO Box; Extended (apartment);
Street; Locality; Region; Postcode; Country. A VCF import parses the first two into real
`postOfficeBox`/`apartment` components which land in `Card.Addresses` correctly — and are then
invisible in the UI and uneditable, because every flat surface drops them. They are also unsearchable,
since `FlattenAddresses` feeds `addresses_flat` from the flat struct.

**And then [T75](119-T75-plain-save-destroys-card-only-data.md) deletes them** on the next plain
save. That ticket stops the deletion; this one is what makes the data reachable in the first place.

## What to build

1. Widen `ContactAddress` with the sub-street fields worth surfacing. Recommend **three**:
   `POBox`, `Apartment` (vCard's "extended address" — the address-line-2 slot), and `Floor`. These
   are the ones the vCard adapters already round-trip and the ones a person actually types. The
   remaining nine kinds (`room`, `building`, `block`, `district`, `subdistrict`, `direction`,
   `landmark`, `number`, `separator`) stay nested-only — they have no editor demand and would make
   the form unusable.
2. Extend all four mapping functions above to carry them, in both directions.
3. Decide and document where the new parts land in `FormatAddress`'s display line and in
   `FlattenAddresses`' search string. Conventional ordering puts PO Box / apartment / floor between
   street and city.
4. `contacts.addresses` is a JSON column, so **no schema migration is needed** — existing rows simply
   lack the new keys and decode to empty strings.

   **A backfill is worth doing anyway, and it recovers real data.** Contacts imported from a VCF
   still hold `apartment`/`postOfficeBox`/`floor` in `Card.Addresses` — the components were parsed
   correctly and only the flat projection dropped them. Re-deriving `contacts.addresses` (and
   therefore `addresses_flat`) from `card` for existing rows pulls that stranded detail back into the
   editable, searchable surface. **Ordering matters**: do this *after*
   [T75](119-T75-plain-save-destroys-card-only-data.md), because until that lands every plain save is
   still deleting the very components the backfill would recover, and any contact already re-saved
   has lost them for good.
5. Round-trip tests against a `database.InitDB` schema (trap #1): a vCard with a populated PO Box and
   extended-address field imports → survives a save → exports back to the same vCard fields.
   Hand-verify per `/CLAUDE.md`.

## Traps

- **Land [T75](119-T75-plain-save-destroys-card-only-data.md) first, or at least be aware of it.**
  Until it lands, any component this ticket newly surfaces is still destroyed by the next plain
  `db.Save`, so the round-trip test in item 5 will fail for reasons that have nothing to do with this
  change.
- **This changes the `MultiValueField`/`AddressFields` editing contract** that `/CLAUDE.md`'s frontend
  conventions call out as the one place the flat `Contact` shape deliberately survives. That is the
  reason T75 explicitly refused to widen `ContactAddress` as its own fix — widening it is fine, but it
  is *this* ticket's deliberate decision, not a side effect of a bug fix.
- **Don't add address parsing.** Splitting `123 Fake St, Apt 456` into components heuristically is a
  different, much worse idea — it would silently restructure data the user typed deliberately. The
  fix is to offer the fields, not to guess.

## Done when

- `ContactAddress` carries PO box, apartment, and floor, and all four mapping functions round-trip
  them.
- A vCard with a populated PO Box and extended-address field imports, saves, and exports back
  unchanged.
- The new parts appear in the formatted display line and in the `addresses_flat` search string.
- `cd backend && go build ./... && go vet ./... && gofmt -l . && go test ./...` green.
