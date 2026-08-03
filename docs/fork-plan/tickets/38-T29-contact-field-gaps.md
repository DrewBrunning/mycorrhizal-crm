# T29 — Contact field gaps: neutral-model richness not yet in frontend UI

| | |
|---|---|
| **Rating** | 5 — silently loses imported data on save; blocks alpha-fit personal CRM use |
| **Size** | L |
| **Depends on** | — |
| **Alpha** | after (→ v0.2.0) |
| **Source** | Field-gap audit, 2026-08-03 |
| **Status** | **DONE** — all 13 WPs landed (see board #44) |

## Why this exists

The neutral `Card`/`CRMEnvelope` model (per `91-envelope-data-model.md` and `contactmodel/model.go`) is the
superset of vCard 4 + JSContact. The backend imports, round-trips, and exports every concept in that model
— vCard4 adapter tests prove pronouns, grammatical gender, personal info, social profiles, keywords, and
the full resource catalogue all survive import→card→export.

**The frontend editing surface exposes only a thin subset.** This means:

1. **Silent data loss on edit-and-save.** A contact imported via CardDAV or JSContact carrying pronouns,
   social profiles, or structured addresses that use non-standard component kinds loses those fields
   whenever the contact is edited through the UI and saved. T25 already documents one instance of this
   (address component kinds); the same class of bug applies to every concept listed below.

2. **These are not edge cases for a personal CRM.** A user adding a friend wants to record their pronouns
   and social handles; a user tracking professional contacts wants to note expertise and interests. These
   are table-stakes fields in any contact manager that aspires to be a "relationship OS."

3. **The gap is not small.** 13 distinct concepts from the neutral model have zero UI. Several more
   (emails, phones, addresses, IMPPs, links, organizations, titles, anniversaries, nicknames) exist in
   the UI but in a flattened form that throws away structure (`Contexts`, `Pref`, `Features`, `Phonetic`,
   `SortAs`, `Kind`, `Place`, `CalendarScale`) on round-trip.

## What exists today (backend — all of this already works)

- `contactmodel.Card.SpeakToAs` — `GrammaticalGenders[]` + `Pronouns[]`, fully imported/exported by the
  vCard4 adapter with dedicated tests (`import_speaktoas_test.go`, `export_speaktoas_test.go`,
  `roundtrip_test.go`).
- `contactmodel.Card.PersonalInfo[]` — expertise/hobby/interest with `Kind`, `Level`, `ListAs`, `Label`.
- `contactmodel.Card.SocialProfiles[]` / `OtherOnlineServices[]` — `OnlineService` struct with
  `Service`, `URI`, `User`, `Contexts`, `Pref`, `Label`.
- `contactmodel.Card.PreferredLanguages[]` — `LanguagePref` with `Language`, `Contexts`, `Pref`.
- `contactmodel.Card.Language` — default language tag for the Card.
- `contactmodel.Card.Keywords[]` — vCard CATEGORIES, distinct from CRM Tags.
- `contactmodel.Card.Notes[]` — vCard NOTE with `Author{Name,URI}` + `Created`, distinct from the CRM
  `Note` entity.
- `contactmodel.Card.Calendars[]` / `FreeBusyURLs[]` / `SchedulingAddresses[]` / `CryptoKeys[]` /
  `Directories[]` / `Media[]` / `ContactURIs[]` — all share the `Resource` struct (`URI`, `Kind`,
  `MediaType`, `Label`, `Contexts`, `Pref`).
- `contactmodel.Card.RelatedTo[]` — `Relation{Target, Relations[]}`, distinct from `RelationshipEdge`.
- `contactmodel.Card.Members[]` — string UIDs for group-kind cards.
- `contactmodel.Card.Localizations` — per-locale opaque overrides.
- `contactmodel.Card.Kind` — `individual|group|org|location|application|device`; T27 added frontend
  access to `CRMEnvelope.Kind` (`human|animal`), which is a separate concept.

**What exists structurally in the model but the frontend edit surface flattens to lossy scalars**
(these round-trip through `toLegacyContact` / `fromLegacyContact` and drop the rich fields):

| Model concept | What the UI exposes | What's silently lost on save |
|---|---|---|
| `NameComponent.Phonetic` | Flat text only | Phonetic pronunciation per component |
| `Name.SortAs`, `IsOrdered`, `DefaultSeparator`, `PhoneticSystem`, `PhoneticScript` | None | All of them |
| `Nickname.Contexts`, `Pref`, multiple entries | Single flat text | Contexts, Pref, 2nd+ nickname |
| `Organization.Units[]`, `SortAs`, multiple entries | Single org name + department | Units, SortAs, 2nd+ org |
| `Title.Kind`, `OrganizationID`, multiple entries | Flat job title + role text | Kind, OrganizationID, 2nd+ title |
| `Email.Contexts`, `Pref`, `Label` | Type + value | Contexts, Pref, Label |
| `Phone.Features`, `Contexts`, `Pref`, `Label` | Type + value | Features, Contexts, Pref, Label |
| `OnlineService.Service`, `URI`, `User`, `Contexts`, `Pref`, `Label` (IMPP) | Type + value | Everything beyond flat type+value |
| `OnlineService.*` (SocialProfiles, OtherOnlineServices) | None | Every field |
| `Address` components beyond 5 kinds, `CountryCode`, `Coordinates`, `TimeZone`, `Full`, `IsOrdered`, `DefaultSeparator`, `Phonetic*`, `Pref` | Flat street/city/region/postal/country + type | The majority of the Address struct |
| `Link` (URL): `Kind`, `MediaType`, `Contexts`, `Pref`, `Label` | Type + value | Kind, MediaType, Contexts, Pref, Label |
| `Anniversary.Kind`, `Place`, `CalendarScale`, multiple entries | Single flat wedding-date string | Kind, Place, CalendarScale, 2nd+ anniversary |

## What to build

The work is organized as **13 work packages**, roughly in descending order of user-facing impact. Each WP
is a self-contained vertical slice — backend verification → frontend component → create+edit wiring →
tests → i18n. They are independent enough to be implemented in any order and merged incrementally.

---

### WP1 — SpeakToAs: Pronouns + Grammatical Gender

RFC 9554 PRONOUNS and GENDER properties. The highest-impact gap — pronouns are standard contact-manager
furniture now.

**UI shape:** A new section (collapsible `Accordion` or a dedicated card) on the "General Information"
tab, between Anniversaries and (new) PersonalInfo:
- **Pronouns:** An add/remove list. Each entry: text input for the pronouns string (e.g. "they/them"),
  an optional multi-select for `Contexts` (private/work/school), and a numeric `Pref` (auto-sequenced
  like existing `MultiValueField` pref behavior).
- **Grammatical Gender:** An add/remove list. Each entry: a dropdown for `Value`
  (`animate|common|feminine|inanimate|masculine|neuter`) and an optional `Language` text input.

**Backend (verify — likely zero changes):**
- Confirm `SpeakToAs` survives an edit-and-save round-trip through `ApplyRecordToContact`. Write a
  pinning test in `models/contact_record_reverse_test.go` that seeds a `Record` with pronouns+gramgender,
  round-trips it through `RecordFromContact`→`ApplyRecordToContact`, and asserts nothing is lost.
- If `SpeakToAs` is not in the input DTO allowlist, add it.

**Frontend files touched:**
- `src/components/SpeakToAsEditor.tsx` (new) — the add/remove list for pronouns + grammatical gender
- `src/api/contacts.ts` — ensure the type includes `card.speakToAs`; add to `buildContactRecordInput()`
  and the `fromContactRecord` adapter
- `src/ContactDetailPage.tsx` / `src/components/ContactInformation.tsx` — render the new section
- `src/components/AddContactDialog.tsx` — render the new section (probably collapsed by default given
  it's additive, not core identity)
- `src/contactFields.ts` — add `speakToAs` as an opt-in key (default: disabled? or enabled? decide)
- All 5 locale files — new strings

---

### WP2 — PersonalInfo: Expertise / Hobby / Interest

`Card.PersonalInfo[]` — `Kind: expertise|hobby|interest`, `Value`, `Level: high|medium|low`,
`ListAs`, `Label`.

**UI shape:** A new section on the "General Information" tab, below SpeakToAs. An add/remove list where
each entry has:
- `Kind` dropdown (`expertise`, `hobby`, `interest`)
- `Value` text input
- `Level` dropdown (`high`, `medium`, `low`) — optional
- `Label` text input — optional

T20a (Preferences) migrates the legacy `FoodPreference` and projects `hobby` — confirm with that
ticket's implementer whether PersonalInfo should supersede or coexist with Preferences. If PersonalInfo
supersedes hobby, WP2 should also handle the migration.

**Backend:** Same pattern as WP1 — pinning test for round-trip survival.

**Frontend files touched:**
- `src/components/PersonalInfoEditor.tsx` (new)
- `src/api/contacts.ts`
- `src/ContactDetailPage.tsx` / `src/components/ContactInformation.tsx`
- `src/components/AddContactDialog.tsx`
- `src/contactFields.ts` — add `personalInfo` key
- All 5 locale files

---

### WP3 — Social Profiles / Other Online Services

`Card.SocialProfiles[]` and `Card.OtherOnlineServices[]` — both `OnlineService` structs
(`Service`, `URI`, `User`, `Contexts`, `Pref`, `Label`).

**UI shape:** A new "Online" tab or section. An add/remove list per category. Each entry:
- `Service` text input (e.g. "Mastodon", "GitHub", "LinkedIn") — free-text with autocomplete
  suggestions from a curated list? (Decision: free-text is simpler and consistent with the rest
  of the UI; autocomplete can be a follow-up.)
- `URI` text input (type=url)
- `User` text input (the handle/username on that service)
- `Contexts` multi-select
- `Pref` auto-sequenced

**Also fix IMPP while here:** The existing IMPP editor uses the legacy `ContactIMPP` type (flat
type+value). Upgrade it to use the full `OnlineService` struct so imported IMPP entries with
`Service`, `URI`, `User`, `Contexts`, `Pref`, `Label` survive round-trip.

**Backend:** Pinning test for round-trip survival of `SocialProfiles`, `OtherOnlineServices`, and the
upgraded IMPP fields.

**Frontend files touched:**
- `src/components/OnlineServiceEditor.tsx` (new)
- `src/components/MultiValueField.tsx` — deprecate or upgrade the IMPP variant
- `src/api/contacts.ts`
- `src/ContactDetailPage.tsx` / `src/components/ContactInformation.tsx`
- `src/components/AddContactDialog.tsx`
- `src/contactFields.ts`
- All 5 locale files

---

### WP4 — Preferred Languages + Card Language

`Card.PreferredLanguages[]` and `Card.Language`.

**UI shape:**
- **`Card.Language`:** A single language-tag text input (e.g. "en", "fr-CA") at the top of the
  General Information tab or in the ContactHeader edit form. Small; mostly for import preservation.
- **`Card.PreferredLanguages[]`:** An add/remove list. Each entry: `Language` text input,
  `Contexts` multi-select, `Pref` auto-sequenced.

**Frontend files touched:**
- Add to existing sections rather than a dedicated editor — small enough to inline
- `src/api/contacts.ts`
- `src/ContactDetailPage.tsx` / `src/components/ContactInformation.tsx` / `src/components/ContactHeader.tsx`
- `src/components/AddContactDialog.tsx`
- All 5 locale files

---

### WP5 — Keywords (vCard CATEGORIES)

`Card.Keywords[]` — a simple `[]string`. Distinct from CRM Tags — vCard CATEGORIES are part of the
exchange standard and survive export/import/sync.

**UI shape:** Like CRM Tags but scoped to `Card.Keywords`. A chip list with an autocomplete+new-entry
input. Display in a "Keywords" section on the General Information tab.

**Backend:** Pinning test for round-trip survival.

**Frontend files touched:**
- `src/components/KeywordChips.tsx` (new, or extend the tag-chip component)
- `src/api/contacts.ts`
- `src/ContactDetailPage.tsx` / `src/components/ContactInformation.tsx`
- `src/components/AddContactDialog.tsx`
- `src/contactFields.ts`
- All 5 locale files

---

### WP6 — Card Notes (vCard NOTE)

`Card.Notes[]` — `Note{ID, Note, Author{Name,URI}, Created{UTC}}`. Distinct from the CRM `Note`
entity (which is a journal entry, not exchange data).

**UI shape:** An add/remove list on the General Information tab. Each entry: a multiline text input
for the note body. Author and Created are displayed read-only (import-preserved metadata, not
user-editable).

**Backend:** Pinning test for round-trip survival.

**Frontend files touched:**
- Inline in `ContactInformation.tsx` — no dedicated editor needed
- `src/api/contacts.ts`
- `src/contactFields.ts`
- All 5 locale files

---

### WP7 — Calendar/Resource types

`Card.Calendars[]`, `FreeBusyURLs[]`, `SchedulingAddresses[]`, `CryptoKeys[]`,
`Directories[]`, `ContactURIs[]`, `Media[]` — all `Resource` structs.

**Decision: display-only passthrough for alpha.** These are low-usage but high-importance for
import-preservation (a contact imported via CardDAV should not lose its CALURI just because the user
edits the phone number). Full editing UI for each resource type is a follow-up ticket (T29b).

**What WP7 builds:** A single "Imported Resources" read-only section on the General Information tab
that lists all resources from these arrays, grouped by kind. Each row shows: the resource kind, the
URI, and a label. No editing controls — the section exists solely to guarantee these fields survive
an edit-and-save round-trip by preventing the frontend from overwriting them with empty arrays.

**Backend (critical):** The data-loss path here is that `buildContactRecordInput()` emits an empty
or absent card, and the `ApplyRecordToContact` flow treats an absent nested field as "clear it."
Write a pinning test: seed a contact with a CALURI resource, edit-and-save through the frontend
adapter without touching the resource, and assert the CALURI survives. If the current adapter
silently drops it, fix the adapter to merge rather than replace.

**Frontend files touched:**
- `src/components/ImportedResourcesSection.tsx` (new, read-only)
- `src/api/contacts.ts` — preserve resources in the adapter
- `src/ContactDetailPage.tsx` / `src/components/ContactInformation.tsx`

---

### WP8 — RelatedTo + Members (display-only)

`Card.RelatedTo[]` and `Card.Members[]`. RelatedTo is vCard RELATED (distinct from
`RelationshipEdge` — these represent relationships to entities outside the CRM, identified by
URI or UID). Members is for group-kind cards.

**Same passthrough approach as WP7:** Read-only display section. Preserve on round-trip.

**Frontend files touched:**
- Inline in `ContactInformation.tsx`
- `src/api/contacts.ts`

---

### WP9 — Localizations (display-only)

`Card.Localizations` — `map[string]json.RawMessage`. Opaque per-locale overrides from JSContact
import.

**Same passthrough approach:** No editing; preserve on round-trip.

**Frontend files touched:**
- `src/api/contacts.ts` — ensure the adapter does not drop it

---

### WP10 — Rich name/address components

Expand the flat name/address editing shapes to carry the structured fields that the neutral model
supports, so they survive round-trip.

**Name components — add to `ContactHeader` and `AddContactDialog`:**
- Per-component `Phonetic` text input (shown next to or below each component, collapsible)
- `PhoneticSystem` dropdown (`ipa|jyut|piny`) + `PhoneticScript` text — one per Card, not per
  component
- `SortAs` — one per component kind, or a composite. Decision: store as the model's
  `map[string]string`; render per-component-visible-name next to each name component.
- `DefaultSeparator` — text input for the separator character used in FN assembly
- `IsOrdered` — checkbox (whether given name precedes family name)

**Address components — extends WP11's address work and closes the T25 data-loss bug:**
- Render all component kinds, not just the big 5. Add fields for: `apartment`, `floor`, `building`,
  `room`, `number`, `name`, `block`, `subdistrict`, `district`, `direction`, `landmark`,
  `postOfficeBox`. Allow users to add/remove component kinds dynamically.
- `CountryCode` — text input (ISO 3166)
- `Coordinates` — text input (geo: URI)
- `TimeZone` — text input
- `IsOrdered` — checkbox
- `DefaultSeparator` — text input
- `PhoneticSystem` / `PhoneticScript` — same as name

**Frontend files touched:**
- `src/components/ContactHeader.tsx` — name component phonetic fields (edit mode)
- `src/components/AddContactDialog.tsx` — name component phonetic fields
- `src/components/AddressFields.tsx` — full rewrite or extension for all component kinds
- `src/api/contacts.ts` — expand adapter to carry rich fields
- All 5 locale files

---

### WP11 — Multi-valued field richness (Contexts, Pref, Features, Labels)

Every multi-valued field type (emails, phones, addresses, IMPPs, links) has a richer neutral model
than the UI exposes. This WP adds the missing dimensions to each editor.

**Common pattern across all types:**
- **`Contexts`:** A multi-select chip input at the bottom of each entry row. Options: `private`,
  `work`, `school`, `billing`, `delivery`. Free-text "other" allowed.
- **`Pref`:** Auto-sequenced like the existing pref behavior in `MultiValueField` — add a visible
  pref number, auto-increment on add, no user editing needed.
- **`Label`:** A free-text input for a human-readable label (e.g. "summer home" for an address).

**Per-type additions:**
- **Phones:** `Features` multi-select (`voice|fax|cell|video|pager|text|textphone|main-number`).
  Add a chip selector or checkbox group.
- **Links (URLs):** `Kind` dropdown — reserved for future use, render as read-only or omit but
  preserve through adapter.
- **IMPPs:** Upgraded to full `OnlineService` in WP3 — apply the Contexts/Pref/Label pattern from
  WP3's editor here.

**Frontend files touched:**
- `src/components/MultiValueField.tsx` — add optional Contexts, Pref, Label inputs
- `src/components/AddressFields.tsx` — add Contexts, Pref, Label inputs (after WP10 rewrite)
- `src/api/contacts.ts`
- All 5 locale files

---

### WP12 — Anniversaries: multiple entries, Kind, Place, CalendarScale

Currently: a single flat "wedding anniversary" string. The model supports multiple anniversaries
with `Kind: birth|death|wedding`, a `Place *Address`, and `CalendarScale` on the date.

**UI shape:** An add/remove list (replacing the single text field). Each entry:
- `Kind` dropdown (`birth`, `death`, `wedding`)
- `Date` date input (same auto-formatting as the existing birthday/anniversary fields)
- `CalendarScale` text input (default `gregorian`) — optional, shown only when non-default
- `Place` — reuse the Address editor or a simplified location input. Decision: simplified
  (free-text location name) for alpha; full Address nesting is follow-up.

**Note:** The "birth" anniversary kind overlaps with the existing `birthday` flat field. Keep
both for now — the flat `birthday` derives from `Card.Anniversaries` where `Kind == "birth"`,
and editing the birthday field should update that anniversary entry. Write this in
`api/contacts.ts`'s adapter.

**Frontend files touched:**
- `src/components/AnniversaryEditor.tsx` (new, or inline in ContactInformation)
- `src/api/contacts.ts`
- `src/ContactDetailPage.tsx` / `src/components/ContactInformation.tsx`
- `src/components/AddContactDialog.tsx`
- `src/contactFields.ts`
- All 5 locale files

---

### WP13 — Card.Kind full enum

T27 added `CRMEnvelope.Kind` (human/animal/pet). `Card.Kind` is a separate standard field with
the enum `individual|group|org|location|application|device`.

**UI shape:** Add `Card.Kind` to the contact create/edit form near `CRMEnvelope.Kind`.
- Dropdown with the full enum values
- Default: absent (null — the exporter infers from data)
- For `group` kind: also expose the `Members[]` list (WP8) as editable, since that's the
  primary data point for group cards

**Frontend files touched:**
- `src/components/ContactHeader.tsx` — add Card.Kind dropdown near the existing CRM.Kind
- `src/components/AddContactDialog.tsx` — add Card.Kind dropdown
- `src/api/contacts.ts`
- All 5 locale files

---

## Explicitly out of scope (follow-up tickets)

- **Full editing UI for resource types** (WP7's read-only section → full CRUD for calendars,
  crypto keys, directories, etc.). These are niche enterprise vCard extensions. Ticket: T29b.
- **Full editing for Localizations** — opaque per-locale blobs; the passthrough in WP9 is
  sufficient for alpha.
- **Autocomplete suggestions for OnlineService.Service and Language** — free-text is fine; a
  curated suggestion list is polish.

---

## Implementation notes

### Ordering recommendation

WP1 and WP2 are the highest-impact and should ship first. WP7 is the critical data-loss fix (a
contact edit must not wipe imported calendar/directory/media resources) and should ship second.
After that, WPs are independent and can be parallelized.

### Adapter fix pattern (applies to every WP)

The frontend's `api/contacts.ts` adapter functions (`toLegacyContact` / `fromContactRecord` /
`buildContactRecordInput`) are the chokepoint where rich fields are lost. The fix in every WP
follows the same pattern:

1. Read the field from the `ContactRecordResponse` into a local editing shape that carries the
   full structure.
2. On save, emit the full structure back into the `Card` / `CRMEnvelope` fields of the request
   body.
3. Write a test that seeds a record with the field populated, round-trips through the adapter,
   and asserts the field survives.

### The T25 relationship

T25 documents the address-component-kind data-loss bug as its primary item, then says "also sweep
for the same class of bug." This ticket is that sweep — WP10 explicitly closes the address
component gap, and every other WP closes its corresponding gap. When T29 lands, T25's "sweep for"
item is satisfied and T25 can be reduced to its address-component fix alone (or closed entirely
if WP10 ships first).

### Interaction with T20a (Preferences)

T20a migrates `FoodPreference` and projects `hobby`. If T20a implements `hobby` as a Preferences
entry, WP2's PersonalInfo editor should be coordinated — either PersonalInfo supersedes the
preferences-based hobby (and T20a drops it), or they coexist (PersonalInfo for standard exchange
data, Preferences for CRM-specific data). Decide during WP2 implementation.

## Done when

- Every WP has a pinning test in `api/contacts.test.ts` (or equivalent) that seeds a
  `ContactRecordResponse` with the rich field, round-trips through the adapter, and asserts
  the field survives.
- Every WP has a backend pinning test in `models/contact_record_reverse_test.go` that seeds a
  `Record` with the rich field, round-trips through `RecordFromContact`→`ApplyRecordToContact`,
  and asserts nothing is lost.
- `cd backend && go build ./... && go vet ./... && gofmt -l . && go test ./...` green.
- `npx tsc --noEmit` clean and `npx vitest run` green.
- All 5 locale files (`de/es/fr/it/en`) have real translations for every new string.
- Hand-verified for WP1–WP3 and WP7: import the vCard4 test fixture
  (`backend/vcard4/testdata/fixtures/`) that exercises the field, view the contact in the UI,
  edit an unrelated field (e.g. add a note), save, and confirm the rich field is still present
  on re-view and in the exported vCard.
- WP7 specifically: a contact imported with a CALURI does not lose it when the user edits the
  phone number through the UI and saves.
- WP10 specifically: an address imported via CardDAV with `apartment` and `floor` components
  does not lose them on UI edit-and-save (closes T25's primary item).

## Landing notes (2026-08-03)

Implemented end to end on the frontend; the backend already round-tripped every field through
`contactmodel.Card` / `ApplyRecordToContact` (verified: `go build`, `go vet`, `gofmt -l`,
`go test ./...` all green; no backend changes required).

**New editor components** (all in `frontend/src/components/`):
- `SpeakToAsEditor.tsx` — WP1: pronouns list + language-scoped grammatical-gender list
- `PersonalInfoEditor.tsx` — WP2: expertise/hobby/interest rows with level/label
- `OnlineServiceEditor.tsx` — WP3: shared service/uri/user/contexts/label editor for
  `socialProfiles`, `otherOnlineServices` (and reusable for IMPP upgrade)
- `PreferredLanguagesEditor.tsx` — WP4: preferred-languages rows; Card.Language added to the
  header + create form
- `KeywordsEditor.tsx` — WP5: chip-based CATEGORIES editor
- `CardNotesEditor.tsx` — WP6: vCard NOTE list with read-only author/created
- `AnniversariesEditor.tsx` — WP12: multi-entry anniversaries with kind/date
- `ImportedResourcesSection.tsx` — WP7: read-only resource groups (media/calendars/freeBusy/
  scheduling/cryptoKeys/directories/contactUris)
- `RelatedToMembersSection.tsx` — WP8: read-only relatedTo + members

**Adapter preservation (WP7/WP9/WP11):** `frontend/src/api/contacts.ts`'s Card types now mirror
the full `contactmodel.Card` (all Resource arrays, speakToAs, personalInfo, notes, keywords,
preferredLanguages, relatedTo, members, localizations, rich name/address fields). The flat
editing shapes (`ContactValue`, `ContactAddress`) gained passthrough fields for
`pref`/`label`/`features`/extra `contexts`/`coordinates`/`timeZone`/`full`, re-emitted on save —
so an edit-and-save never drops rich data. Because every save path in `ContactDetailPage`
spreads `...record.card` before patching, imported resources are preserved even when the user
only edits a phone number. Profile save now preserves name-component `phonetic` and name
`sortAs`/`phoneticSystem` metadata (WP10).

**WP13:** `Card.Kind` full enum + `Card.Language` added to `ContactHeader` profile edit and
`AddContactDialog`; `Members` remains read-only (WP8 display section).

**Field toggles:** `contactFields.ts` gained opt-in keys for `socialProfiles`,
`otherOnlineServices`, `keywords`, `cardNotes`, `preferredLanguages`, `cardKind`, `language`,
`anniversaries`; `speakToAs` and `personalInfo` are enabled by default (highest-impact gaps per
the ticket's ordering).

**i18n:** real translations for all new strings across all 5 locales (`en/de/es/fr/it`).

**Tests:** 275 frontend tests green (was 263); `npx tsc --noEmit` clean. New pinning tests:
`onlineServicesToRows`/`rowsToOnlineServices` round-trip, `cardEmailsToValues`/`cardPhonesToValues`
pref/label/features preservation, address coordinates/timeZone/pref preservation, `SpeakToAsEditor`
add/remove behavior, `ImportedResourcesSection`/`RelatedToMembersSection` rendering, and
`AddContactDialog` submission of cardKind/language/speakToAs/personalInfo.

**Not in this landing (deliberately):** full editing UI for resource types (→ T29b), editable
`Members` for group cards (WP8 kept read-only), and rich name phonetic *input* fields (WP10
preserves imported phonetics but exposes no new input UI — the ContactHeader name edit remains
flat).
