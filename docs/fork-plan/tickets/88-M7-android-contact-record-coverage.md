# M7 — Android contact record: the editor covers 8 of ~30 field groups

| | |
|---|---|
| **Platform** | Android |
| **Rating** | 4 — the contact record is the product; editing it is the app's primary job |
| **Size** | L — a new shared editor plus ~20 field groups; the largest Android ticket. Consider slicing by field group. |
| **Source** | Post-M1 review pass, 2026-08-11 — found by diffing `ContactFormViewModel`'s state against the `Card`/`CRMEnvelope` model |
| **Depends on** | Nothing. The API client, the nested `Card` model, and the merge-on-edit write path all already exist. |
| **Status** | Scoped, not started. |

This is the **depth** gap inside the contact record. The **breadth** gap — whole web screens with
no Android equivalent — is [M8](89-M8-web-android-parity-audit.md).

## Why this exists

M1's landing note reports item 6 ("contact create/edit — full Card write path") as complete, and the
write *path* genuinely is: POST/PUT with the §2.6 wrapped-POST unwrap, Room caching, backend-aligned
validation. What is not complete is the **form's coverage of the model**. The editor
(`ContactFormViewModel.ContactFormState`) models eight things:

> given name · surname · nickname · emails[] · phones[] · birthday · notes · circles

The neutral `Card` carries roughly thirty field groups, and `CRMEnvelope` carries four. So on
Android a contact's address, organization, job title, online-service handles, links, pronouns and
"how we met" cannot be created or changed at all — several of them are *displayed* on the detail
screen, which makes the gap read as a bug rather than as missing scope.

**This is not a data-loss risk.** `ContactFormViewModel.buildInput` merges onto the loaded record
rather than rebuilding it, so the backend's full-overwrite PUT preserves everything the form does
not model. That was a deliberate, correct call. It just means the ceiling on Android is "don't
break it", not "edit it".

## The three tiers

### Tier 1 — rendered on the detail screen, but read-only

These already have a display surface (`ContactDetailScreen`'s `SectionCard`s), so the gap is
visible to the user every time they tap edit and the field isn't there. Highest priority.

| Field group | Card path | Detail section |
|---|---|---|
| Addresses | `card.addresses[]` | Address |
| Organizations | `card.organizations[]` | Organization |
| Job titles | `card.titles[]` | Organization |
| IMPP / social / other online services | `card.imppAddresses[]`, `socialProfiles[]`, `otherOnlineServices[]` | Online services |
| Links | `card.links[]` | Links |
| Personal info | `card.personalInfo[]` | Personal information |

### Tier 2 — in the model, surfaced nowhere

Neither editable nor displayed. Decide per group whether it earns a mobile surface at all — some
of these are legitimately desktop-only concerns, and saying so explicitly is a valid outcome.

`card.speakToAs` (pronouns — note `RecordForContact` is the only read path that returns it, per
`/CLAUDE.md` backend trap 3) · additional `card.anniversaries[]` beyond birthday ·
`card.preferredLanguages[]` · `card.keywords[]` · `card.calendars[]` ·
`card.schedulingAddresses[]` · `card.freeBusyUrls[]` · `card.cryptoKeys[]` · `card.directories[]` ·
`card.contactUris[]` · `card.relatedTo[]` (distinct from the relationship-edge graph) ·
`card.media[]` beyond the photo.

### Tier 3 — CRM envelope

`crm.circles` is editable (as a comma-separated text field, which is itself worth revisiting).
`crm.how_we_met`, `crm.work_information` and `crm.contact_information` appear in **neither** the
form nor the detail. `how_we_met` in particular is a real relationship-OS field, not metadata —
`91.10`/the relationship-type registry treat it as first-class, and the web surfaces it.

## The design — decided 2026-08-11

The web edits these through `MultiValueField`/`AddressFields`, which operate on the **flat**
`Contact` shape — one of only two places `/CLAUDE.md` allows the flat type to survive. Android has
no equivalent component and its form is built directly on the nested model, so there is nothing to
port; the component has to be designed for the nested types.

### 1. One generic editor driven by a per-type spec — not six bespoke sections, not one sealed union

The nested types already share a shape. From `core/model/.../CardTypes.kt`:

| Type | value field | type field | also carries |
|---|---|---|---|
| `Email` | `address` | `label` | `id`, `contexts`, `pref` |
| `Phone` | `number` | `label` | `id`, `contexts`, `pref`, `features` |
| `OnlineService` | `uri` / `user` | `label` | `id`, `contexts`, `pref`, `service` |
| `Address` | `components[]` | — | `id`, `contexts`, `pref`, `full`, … |
| `Organization` | `name` | — | `id`, `units` |
| `Title` | `name` | `kind` | `id`, `organizationId` |
| `PersonalInfo` | `value` | `kind` | `id`, `level`, `listAs`, `label` |

Three of them (`Email`, `Phone`, `OnlineService`) are the same editor with different labels and
keyboard types. So:

```kotlin
// core/ui/components/MultiValueEditor.kt
interface MultiValueSpec<T> {
    fun value(item: T): String
    fun withValue(item: T, value: String): T   // MUST be item.copy(...)
    fun type(item: T): String?                 // maps to `label` or `kind`
    fun withType(item: T, type: String?): T
    fun pref(item: T): Int?
    fun withPref(item: T, pref: Int?): T
    fun blank(): T
    val typeOptions: List<String>              // hardcoded mirror of the backend
    val keyboardType: KeyboardType
}

@Composable fun <T> MultiValueEditor(
    items: List<T>, spec: MultiValueSpec<T>, onChange: (List<T>) -> Unit, label: String,
)
```

Each row is `value field + type dropdown + preferred toggle + delete`, with an add-row beneath.
`EmailSpec`/`PhoneSpec`/`OnlineServiceSpec` are ~15 lines each. `Title` and `PersonalInfo` reuse it
with `type` bound to `kind` instead of `label`.

**`Address` gets its own `AddressEditor`**, not a spec — it is structurally different
(`components[]`, not a scalar). Mirror the web's `AddressFields`, and **coordinate with
[T79](123-T79-flat-address-projection-too-narrow.md)/[T80](124-T80-web-address-editor-line-two.md)**
so both platforms agree on which component kinds are editable. Also read
[T67](111-T67-android-address-import-parsing.md) first: the registry kinds are `name`/`locality`,
**not** `street`/`city`, and getting that wrong is what made the device-import path lose addresses.

`Organization` is a bare string in practice — a plain field, no editor needed.

### 2. The non-negotiable rule: edit entries in place, never rebuild them

This is the part that matters more than the component shape, and it is the reason
[T81](125-T81-android-contact-edit-corrupts-phone-email-metadata.md) was split out of this ticket:
today's form holds `List<String>` and *reconstructs* the nested objects on save, destroying `id`,
`contexts`, `pref`, and `label` every time.

The form state must hold the **real loaded objects** (`List<Email>`, not `List<String>`), and every
edit must go through `.copy(...)` — never a fresh constructor. That preserves `id` (the backend's
identity handle for the entry) plus every field the editor chose not to surface. It is the
client-side mirror of `/CLAUDE.md` backend traps #2/#3 and of
[T75](119-T75-plain-save-destroys-card-only-data.md): *a lossy projection must never overwrite the
authoritative record.*

Bake it into the interface — `withValue` returning a `copy` rather than a constructor is the whole
guarantee — and pin it with a test asserting that an entry's `id` and `contexts` survive a
load → edit → save round-trip.

### 3. `pref` is a list-level invariant, not a per-row one

`preferred` on phone/email is a shipped web feature ([T58](77-T58-preferred-phone-email-ui.md)) with
no Android counterpart. `pref = 1` means preferred; **at most one entry per list may hold it**, so
setting it on one row must clear it on the others. That is list-level logic and belongs in
`MultiValueEditor`, not in each spec — otherwise every future spec gets a chance to forget it.

### 4. Type-option lists are hardcoded mirrors

`typeOptions` duplicates backend `oneof` validators, exactly like the web's copies. There is no
dynamic type-list endpoint in this codebase, by design (`/CLAUDE.md` frontend trap #4). Add the same
"must stay in sync" comment the web copies carry.

## Suggested order

1. The multi-value editor component + emails/phones round-tripping type/label/preferred (fixes a
   live lossy edit, and unblocks everything else).
2. Tier 1 addresses and organizations/titles — the two most-used.
3. Tier 3 CRM envelope fields — cheap, they are plain strings.
4. Tier 1 online services and links — these interact with the mobile link-action resolver (§7.6),
   so editing a handle must keep the resolved chips working.
5. Tier 1 personal info.
6. Tier 2, per group, with an explicit "not on mobile" verdict recorded for the ones that don't
   earn a surface.

## Done when

- Each tier-1 group is creatable and editable on Android, and a round-trip (edit on Android → read
  on web) preserves type, label and `preferred` rather than flattening them.
- Emails and phones no longer discard per-entry metadata on edit — pinned by a test that fails if
  they do.
- Every tier-2 group has a recorded verdict: implemented, or explicitly out of scope for mobile.
- New user-facing strings land in `core/ui/src/main/res/values{,-de,-es,-fr,-it}/strings.xml` with
  real translations.
- `./gradlew testDebugUnitTest lintDebug assembleDebug` green
  (`ANDROID_HOME=/home/drew/Android/Sdk JAVA_HOME=/home/drew/android-studio/jbr`), new tests
  hand-verified per `/CLAUDE.md`.
- On-device verification on the Pixel 8a, matching how every M1 phase was signed off.

---

## Implementation contract (added 2026-08-12)

### Endpoints

| Need | Route | In `ApiClient`? |
|---|---|---|
| Save the whole record | `PUT /contacts/:id` | Yes (`updateContact`) |
| Load it | `GET /contacts/:id` | Yes (`getContact`) |

**No new endpoints for the Card field groups** — addresses, organizations, titles, online services,
links and personal info all live on the nested `Card` and ride the existing contact update. That is
what keeps this ticket about UI and state shape rather than API surface.

**Resolved 2026-08-12**: every field group this ticket lists is a `Card`/`CRM` member, so all of
them ride `updateContact`. Custom fields are a genuinely separate feature (T6/T7) reached through
`GET/PUT /contacts/:id/field-values` and `GET /field-definitions` — **none of which are in
`ApiClient`** — and they are **out of scope here**, tracked as
[T84](128-T84-android-custom-field-values.md). Don't let them creep into this ticket.

### Test cases

1. **`id` and unshown fields survive a round-trip** — load an entry carrying `id`, `contexts` and
   `pref`, edit only its value through `MultiValueEditor`, save, and assert all three are intact.
   This is the guarantee the whole `withValue → copy()` interface exists to provide, and the bug
   [T81](125-T81-android-contact-edit-corrupts-phone-email-metadata.md) fixes for emails/phones.
2. **`pref` is exclusive** — setting preferred on one entry clears it on every other entry in that
   list. List-level, so test it at the editor, not per spec.
3. **New rows get their default type; loaded rows keep theirs** — the `cell` default must not
   overwrite a loaded `work` label.
4. **Parameterize 1–3 across `EmailSpec`, `PhoneSpec` and `OnlineServiceSpec`.** The claim is that one
   editor serves all three; per-type tests would not establish it.
5. **Address editor** — the registry kinds are `name`/`locality`, **not** `street`/`city`
   ([T67](111-T67-android-address-import-parsing.md) shipped broken on exactly that). Assert the kinds
   emitted, not just the values.
6. **Type-option lists match the backend's `oneof` validators** — hardcoded mirrors with no dynamic
   endpoint, by design.

### Gate

- `./gradlew testDebugUnitTest`, `./gradlew lintDebug`, `./gradlew assembleDebug` — the exact three
  steps `.github/workflows/android-tests.yml` runs. CI has been green since M1's review pass; keep it.
- Every new user-facing string in all five locales (`values`, `values-de/es/fr/it`). M1's review pass
  had to retrofit ~80 unlocalized strings — don't rebuild that debt.

### Test conventions (this repo, not generic)

JUnit4 + MockK (`mockk`/`coEvery`) + Turbine + `runTest` with `MainDispatcherRule`. ViewModel tests
mock the repository — `feature/contacts/.../ContactListViewModelTest.kt` is the reference. New
`ApiClient` methods get a MockWebServer test in `core/network` — `ApiClientTest.kt` is the reference.
Hand-verify per `/CLAUDE.md`: break the code, confirm the new test fails, restore.
