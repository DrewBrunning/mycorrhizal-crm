# M7 — Android contact record: the editor covers 8 of ~30 field groups

| | |
|---|---|
| **Rating** | 4 — the contact record is the product; editing it is the app's primary job |
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

## The design question to settle first

The web edits these through `MultiValueField`/`AddressFields`, which operate on the **flat**
`Contact` shape — one of only two places `/CLAUDE.md` allows the flat type to survive. Android has
no equivalent component and its form is built directly on the nested model.

So before building six editors, decide: a reusable Compose multi-value editor (a
`MultiValueField`-alike over `List<Email>`/`List<Phone>`/`List<Address>` with type/label/preferred
per entry), or six bespoke sections? The reusable component is almost certainly right — emails and
phones already need it, and today they are a `List<String>` that silently discards each entry's
type, label and `preferred` flag on edit. That discard is worth confirming and fixing as part of
Tier 1 whichever way the decision goes.

Note also that `preferred` on phone/email is a shipped web feature ([T58](77-T58-preferred-phone-email-ui.md))
with no Android counterpart.

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
