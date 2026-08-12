# T67 — Android device-contacts import loses address data

| | |
|---|---|
| **Platform** | Android |
| **Rating** | 4 — silent data loss on a shipped, real-data import path |
| **Size** | S (~half day to a day) |
| **Depends on** | Nothing. Related to [T75](119-T75-plain-save-destroys-card-only-data.md) — see Bug C below — but neither blocks the other. |
| **Status** | TO BE DONE |
| **Source** | Testing notes, 2026-08-11: "Android contact import loses address information (it's not parsed correctly)" |

## Why this exists

Android's device-contacts import (T57, shipped as part of M1 Phase 5) reads
`ContactsContract.CommonDataKinds.StructuredPostal` rows and loses address data for almost every
contact that has one, via three compounding bugs — all in `android/feature/import/`, none on the
backend or web.

**Bug A — wrong column read as the formatted address**
(`DeviceContactsReader.kt:119-123`). Android's real `StructuredPostal` column layout is
`DATA1=FORMATTED_ADDRESS, DATA4=STREET, DATA7=CITY, DATA8=REGION, DATA9=POSTCODE, DATA10=COUNTRY`.
The reader treats `row.data9` as the formatted address string — but `DATA9` is POSTCODE, not
`FORMATTED_ADDRESS` (`DATA1`, fetched but never read). Since Android auto-populates both fields for
any address entered via the stock Contacts app, the "address" captured is frequently just the raw
postal code, with street/city/region/country silently discarded whenever postcode is non-blank.
This mistake also exists in the design spec itself
(`docs/fork-plan/tickets/67-M1-mobile-android-app.md:1698`), so the implementation faithfully copied
it. `DATA10` (COUNTRY) is never queried at all — country is unconditionally lost regardless.

**Bug B — invalid `AddressComponent.Kind` values** (`DeviceContactMapper.kt:79-91`). Whatever string
survives Bug A gets comma-split and assigned kinds `"street"`/`"city"` positionally — but this
codebase's registry uses `"name"` (street) and `"locality"` (city), confirmed both backend
(`backend/contactmodel/model.go:167-176`, `backend/models/contact_record_reverse.go`) and web
(`frontend/src/components/ContactInformation.test.tsx:242`). No `"postcode"` kind is produced either.

**Bug C — the invalid kinds vanish server-side without complaint.** `contactAddressFromNeutral`
(`backend/models/contact_record_reverse.go:283-303`) switches on `comp.Kind` with no `default` case,
so a component kinded `"street"` or `"city"` — exactly what Android emits — matches nothing and is
dropped when the flat `ContactAddress` is built.

**Re-scoped 2026-08-11.** The original draft called this a backend bug to be fixed alongside A and
B. It isn't, quite: dropping `"street"`/`"city"` is *correct* handling of input that is invalid
under the RFC 9553 registry, and the fix belongs on the Android side (Bug B). Two separate real
backend issues were found underneath it during grooming, and both are tracked elsewhere so this
ticket stays Android-only:

- Address components that *are* valid but have no flat-projection slot (`apartment`, `floor`,
  `postOfficeBox`, …) are destroyed on a later save — a genuine data-loss bug, filed as
  [T75](119-T75-plain-save-destroys-card-only-data.md).
- `AddressComponent.Kind` is never validated on write, which is *why* Android's invalid kinds
  produce a `201` and no error. Noted as a follow-up candidate in T75's "Related" section.

**Net effect for this ticket**: for a typical fully-structured device address, the only component
Android produces is `{kind: "street", value: "<postcode>"}` — an invalid kind carrying the wrong
value — and it is then correctly discarded server-side. The contact ends up with no address at all
in the common case. Fixing A and B on the Android side resolves that; no backend change is required.

A secondary loss: address `TYPE` (home/work) is fetched (`row.data2`) but never used for addresses —
unlike phones, which do retain type/label — so it's discarded unconditionally even before the above.

**Scope**: Android-only. The web VCF/CSV import path (`backend/services/import_service.go`) parses
vCard `ADR`'s well-defined 7-part structure directly into named components with no column-index
ambiguity, and is unaffected.

**No existing coverage**: `DeviceContactMapperTest.kt` never exercises a non-empty `addresses` list,
and there's no test for `DeviceContactsReader`'s `StructuredPostal` column extraction — why this
shipped unnoticed. Not previously documented in T49/T50 (web VCF-import bugs), M1's landing notes, or
M5's hardening review.

## What to build

1. In `DeviceContactsReader.kt`'s `readDataRows`, query `DATA3` (LABEL) and `DATA10` (COUNTRY) in
   addition to the columns already read. Stop treating `DATA9` as "formatted" — use `DATA1`
   (`FORMATTED_ADDRESS`) only as a display fallback, and carry the real structured fields through
   directly: street=`DATA4`, city=`DATA7`, region=`DATA8`, postcode=`DATA9`, country=`DATA10`.
2. Change `DeviceContact.addresses` from `List<String>` to a small structured type (street / city /
   region / postcode / country / type), removing the need for `DeviceContactMapper.splitAddress`'s
   positional guessing entirely.
3. In `DeviceContactMapper.kt`, map to `AddressComponent` using the registry's real kinds — `"name"`,
   `"locality"`, `"region"`, `"postcode"`, `"country"` — not `"street"`/`"city"`.
4. Carry address `TYPE` (home/work) through the same way phones already do, instead of discarding it.
5. Add unit tests for both the reader's column extraction and the mapper's address output (currently
   zero coverage). Hand-verify per `/CLAUDE.md`: break the parsing, confirm the new test fails,
   restore. Given `DeviceContactsReader` touches the real `ContentResolver`/`ContactsContract` and
   this repo has no `androidTest` tier yet (per M5 §6), also do a manual on-device check with a real
   structured address entered via the stock Contacts app.

## Traps

- Don't just fix the column read (Bug A) and stop — Bug B means even a correct extraction is still
  thrown away server-side, because the kinds it is sent under are invalid. A and B must land
  together or the observable behavior won't change at all.
- **The registry kinds are `name` and `locality`, not `street` and `city`.** Verify against
  `contactmodel/model.go:167-176` rather than intuition; the intuitive names are the wrong ones, and
  the backend will accept them silently (see Bug C).
- `docs/fork-plan/tickets/67-M1-mobile-android-app.md:1698`'s column table has the same DATA9
  mislabeling this ticket is fixing in code — worth a corrective note there too so a future reader
  doesn't re-copy the mistake.

## Done when

- A device contact with a full structured address (street, city, region, postcode, country, and a
  home/work type) imports on Android with all fields intact, hand-verified on a real device.
- New unit tests cover the reader's column extraction and the mapper's kind assignment, and were
  confirmed to fail against the old code before the fix.
- `./gradlew testDebugUnitTest`, `./gradlew lintDebug` and `./gradlew assembleDebug` green —
  the three steps `.github/workflows/android-tests.yml` actually runs.
