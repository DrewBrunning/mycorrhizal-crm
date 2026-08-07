# T44 — Link field type registry doesn't reach the Social Profiles / Other Services / Instant Messaging editors

| | |
|---|---|
| **Rating** | 4 — the registry (Settings → "Contact field types") is effectively undiscoverable from the editor you'd actually use it in |
| **Size** | M |
| **Depends on** | [T34](43-T34-contact-field-linking.md) (done — owns `LinkFieldType`, `OnlineServiceEditor`, and explicitly deferred IMPP as a follow-up — see its landing note) |
| **Alpha** | n/a — frontend-only, no schema/API change |
| **Source** | Real-world usage report, 2026-08-06: "Social Profiles doesn't show me link field types (nor does Instant Messaging) — both show me the classic form entry to manually type in URL fields" |

## Why this exists

T34 built a per-user `LinkFieldType` registry (name → URL template → icon) and wired it into
**read-only display resolution**: `ContactInformation.tsx:139-142` fetches
`useLinkFieldTypes()` whenever Social Profiles/Other Services are enabled, and
`renderOnlineServices` resolves a saved entry's `service` string against the registry by exact
case-insensitive name match (`utils/linkResolution.ts:99-125`) to build a clickable link.

But the *editor* the user actually types into never sees the registry at all:

- **Social Profiles / Other Online Services** (`ContactInformation.tsx:583-619`) both render
  `<OnlineServiceEditor showService />`. That component's service field is a plain, unassisted
  `TextField` (`OnlineServiceEditor.tsx:70-77`) — no `Autocomplete`, no icon preview, no awareness
  of `linkFieldTypes` at all. To get link resolution to work, a user has to already know and type
  the *exact* registry name (`WhatsApp`, not `whats app` or `Whatsapp Messenger`) with nothing in
  the UI telling them so or listing what names exist.
- **Instant Messaging** doesn't use `OnlineServiceEditor` at all — it's wired to the generic
  `MultiValueField` with `freeTextType` (`ContactInformation.tsx:628-629`), which only knows a flat
  type/value shape. It discards `Service`/`User` entirely (only the raw URI value survives), so an
  IMPP entry can never be registry-matched even in principle — T34's own landing note documents
  this as a deliberate scope cut, not an oversight: "Making IMPP registry-matchable would mean
  rebuilding its editor around `OnlineServiceEditor` instead of `MultiValueField`... left as a
  follow-up rather than folded into an already-large ticket."

Net effect: the Settings CRUD screen for link field types currently has no forward path into the
data users actually enter — it only matters retroactively, and only for two of the three fields it
was built for.

## What to build

### 1. Wire the registry into `OnlineServiceEditor`'s service field

- Add an optional `linkFieldTypes?: LinkFieldType[]` prop to `OnlineServiceEditorProps`
  (`OnlineServiceEditor.tsx:18-26`).
- Replace the plain service `TextField` (`OnlineServiceEditor.tsx:70-77`) with an
  `Autocomplete freeSolo` sourced from `linkFieldTypes.map(t => t.name)` — `freeSolo` so an
  unregistered/one-off service name still works exactly as it does today. Follow the existing
  `contexts` `Autocomplete` in the same file (`OnlineServiceEditor.tsx:96-107`) as the pattern to
  copy.
- Show each option's icon in the dropdown via `resolveLinkFieldTypeIcon` (from
  `linkFieldTypeIcons.ts`, see [T43](52-T43-link-field-type-custom-icons.md) if that ticket hasn't
  landed yet — either order is fine, they don't conflict).
- Pass `linkFieldTypes` down from `ContactInformation.tsx`'s existing `useLinkFieldTypes()` call
  (already fetched at `ContactInformation.tsx:139-142`) into both `OnlineServiceEditor` call sites
  (`ContactInformation.tsx:591`, `610`).

### 2. Route Instant Messaging through `OnlineServiceEditor` instead of `MultiValueField`

- `OnlineServiceEditor` already has a `uriOnly` prop seemingly built for exactly this
  (`OnlineServiceEditor.tsx:24-25, 40, 80, 87`) — confirm its current behavior matches what IMPP
  needs (service + URI, no separate `user`/handle field) and use it:
  `<OnlineServiceEditor uriOnly label={...} value={...} onChange={...} linkFieldTypes={...} />`
  in place of the current `MultiValueField` block (`ContactInformation.tsx:621-633`).
- This is a real shape change for IMPP data — check `cardImppToValues`/`valuesToCardImpp`
  (`api/contacts.ts`) against `onlineServicesToRows`/`rowsToOnlineServices` (used by
  `OnlineServiceEditor` today for the other two fields) and decide whether IMPP needs its own
  adapter pair or can reuse the existing ones; IMPP's `CardOnlineService` shape should already line
  up since T29 established it, but the *display* path (`renderUriValueList`) also needs updating to
  match `renderOnlineServices`'s registry-aware rendering, not just the editor.
- Confirms/updates T34's landing note, which explicitly flagged this exact gap as a follow-up.

## Traps

- **`freeSolo` must stay free-solo.** Plenty of real services won't be in a user's registry
  (a one-off "Company Slack" or a niche platform) — the Autocomplete is a convenience/discovery aid
  over the registry, not a hard enum. Don't make it a closed `Select`.
- **Case-insensitive match, not exact.** `resolveOnlineServiceLink`'s registry lookup is already
  case-insensitive (`linkResolution.ts:99-125`) — the Autocomplete should suggest the registry's
  canonical casing, but shouldn't silently rewrite what the user typed if it differs only in case
  (that's exactly what the resolver already tolerates at render time).
- **Changing IMPP's editor is the highest-risk part of this ticket** — per `CLAUDE.md`'s frontend
  trap 8 territory (shape mismatches), verify the full round-trip (create → save → reload → edit
  again) preserves existing IMPP data, and specifically test a contact whose IMPP entries predate
  this change (imported via CardDAV/vCard, where `Service` may be empty/absent).
- **i18n**: any new label/placeholder text needs real translations in all 5 locales, not English
  placeholders.
- Don't regress T34's copy-button/tappable-link work on Social Profiles/Other Services or IMPP
  while touching these editors.

## Done when

- `npx tsc --noEmit` clean, `npx vitest run` green, including: a test that the Social
  Profiles/Other Services Autocomplete offers the user's registry entries with icons; a test that
  typing an unregistered name still saves correctly (`freeSolo` preserved); IMPP round-trip tests
  (create/edit/reload) covering both a registry-matched service and a pre-existing entry with no
  `Service` value.
- Hand-verified in a real browser: adding a WhatsApp entry via the Social Profiles editor now shows
  WhatsApp as a suggested option with its icon while typing; saving it resolves to a working link
  exactly as it does today; Instant Messaging now offers the same Autocomplete and a saved entry
  resolves through the registry the same way Social Profiles does.
- All 5 locale files have real translations for any new/changed strings.
- T34's landing note updated (or this ticket's own landing note cross-linked from it) since this
  closes the "IMPP is not routed through the registry" gap it explicitly called out.

## Landing note

Landed on `feature/t44-link-field-type-registry-in-editors`. Frontend-only, no schema/API change.

**`OnlineServiceEditor` service field is now a registry-fed freeSolo `Autocomplete`.**
Added an optional `linkFieldTypes?: LinkFieldType[]` prop; the service field went from a plain
`TextField` to an `Autocomplete freeSolo` sourced from the registry's names, rendering each
option's icon via `resolveLinkFieldTypeIcon` (following `LinkFieldTypesSettings.tsx`'s icon-picker
pattern, including the `renderOption` key-destructure that avoids React's key-spread warning).
`freeSolo` stays free-solo: `onInputChange` (reason `'input'`) persists typed text
keystroke-by-keystroke exactly like the old TextField, and `onChange` accepts both a selected
`LinkFieldType` object (stores `.name`) and a bare string — an unregistered/one-off service name
round-trips unchanged. Without the prop the field is exactly the old free-text input.

**Instant Messaging is routed through `OnlineServiceEditor` (its long-dormant `uriOnly` prop's
first real consumer)** instead of `MultiValueField`/`cardImppToValues`/`valuesToCardImpp`.
`uriOnly` gives service + Address (no `user` field), exactly the IMPP shape. IMPP now reuses the
`onlineServicesToRows`/`rowsToOnlineServices` adapter pair the other two fields use — no new
adapter, and a pre-existing IMPP entry with no `Service` (the vCard/CardDAV import case)
round-trips unchanged. The IMPP display path moved from `renderUriValueList` to the same
registry-aware `renderOnlineServices` the other two fields use, so an IMPP entry resolves through
the user's registry the same way Social Profiles does (full-URI entries still link directly via
`resolveOnlineServiceLink`'s existing first branch). `renderUriValueList` remains, now used only by
the raw `links` (Card.Links) field, which has no `service`/`user` to registry-match.
`renderOnlineServices` gained a `copyLabel` param (the old one hardcoded `contacts.socialProfiles`,
wrong for IMPP); all three call sites pass their own field label.

**Registry fetch gate widened** to include `imppAddresses` — previously only
`socialProfiles`/`otherOnlineServices` triggered the lazy `ListLinkFieldTypes` fetch, so a user
who enabled only IMPP would have gotten an empty Autocomplete. Still lazy (IMPP isn't in
`DEFAULT_ENABLED_CONTACT_FIELDS`), so accounts that never touch the registry cost nothing.

**No new strings** — the editors reuse `contacts.onlineServices.service`/`.impp`/`.uri`/`.user` and
`contacts.impps`, all already real translations in all 5 locales.

**Tests:** `OnlineServiceEditor.test.tsx` (new): registry options render with their resolved icon
path; selecting an option stores the canonical name; typing an unregistered name saves
(`freeSolo` preserved); `uriOnly` hides `user`; editing preserves `id`/`user`/`label`/`contexts`/
`pref`. `ContactInformation.test.tsx` (extended): IMPP display links a full URI directly; the IMPP
editor is `OnlineServiceEditor` in `uriOnly` mode and preserves a pre-existing no-service entry;
the Social Profiles editor offers the registry with icons once the fetch lands. All unit tests
hand-verified (reverting `uriOnly` and the Autocomplete respectively fails the new tests).
`linkFieldTypeEditors.spec.ts` (new e2e): toggles the shared `TEST_USER`'s
`/users/enabled-contact-fields` to include the three fields for the duration of each test and
restores in a finally block (serialized via `test.describe.configure({ mode: 'serial' })` — with
`fullyParallel: true` the file's own tests raced each other's toggle/restore and corrupted the
setting, caught by checking the post-run state); covers the Social Profiles editor offering a
custom type with its icon and saving a working link, freeSolo saving an unregistered name, and
IMPP preserving a pre-existing `xmpp:` entry while gaining a registry service. Full e2e suite
(101 tests) green, including the parallel run leaving `TEST_USER`'s setting restored to null.
