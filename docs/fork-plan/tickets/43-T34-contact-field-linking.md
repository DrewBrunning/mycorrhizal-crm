# T34 — Tappable contact fields: tel:/sms:/mailto:, copy buttons, configurable messaging/social link types, address geo-links

| | |
|---|---|
| **Rating** | 5 — this is core "reach this person from your phone" functionality for a personal CRM |
| **Size** | L |
| **Depends on** | [T29](38-T29-contact-field-gaps.md) (SocialProfiles/OtherOnlineServices/IMPP already carry the `OnlineService` shape this reuses) |
| **Alpha** | n/a — real data exists. New `LinkFieldType` table is additive/new, nothing existing changes; hand-written migration, real-DB test, per `CLAUDE.md` |
| **Source** | v0.2.0-alpha real-world testing |
| **Status** | **DONE** — see landing note at the bottom of this file |

## Why this exists

Right now, phone numbers, emails, messaging handles, social profiles, and other links are inert
text — nothing is tappable, and on mobile, selecting text to copy a phone number or handle is
fiddly. This ticket makes contact fields *actionable*: tap to call/text/email, tap to open a
social/messaging profile, and a copy button on everything as the universal fallback (since
tapping a link *takes an action* rather than letting you select-and-copy on mobile).

## Decisions made (confirmed with the user)

- **Phone and email are hardcoded, not part of any configurable registry** — they're universal
  enough to not need per-user configuration, unlike messaging/social services.
- **Messaging and social services are a user-configurable registry**, seeded with sensible
  defaults, editable/extendable per-user — closer to Monica's "Contact field types" (name,
  protocol, actions) than this repo's usual hardcoded-enum convention. This is a deliberate
  exception to `CLAUDE.md`'s frontend-trap-4 convention (hardcoded mirrors of backend `oneof`
  enums) — noted here so it doesn't read as an inconsistency later.
- **Every field gets a copy button** (`mdi-content-copy`), tappable or not, for consistency and
  because mobile text selection is the actual pain point being solved.

## What to build

### 1. Phone numbers (hardcoded — no registry)

Behavior depends on the phone's existing `Type` (`CONTACT_TYPE_OPTIONS` in `contactFields.ts`:
`home | work | cell | fax | other`):

| Type | `tel:` (call, `mdi-phone`) | `sms:` (text, `mdi-message-text-outline`) | Copy (`mdi-content-copy`) |
|---|---|---|---|
| `cell` (mobile) | ✅ | ✅ | ✅ |
| `home` / `work` / `other` | ✅ | ❌ | ✅ |
| `fax` | ❌ | ❌ | ✅ (copy only — a fax number is not callable or textable) |

### 2. Email (hardcoded — no registry)

The email value itself is the tappable element (`mailto:`) — not a separate button, since email
only has one action. Copy button alongside it regardless.

### 3. Universal copy button

Every displayed contact field value — whether or not it's otherwise tappable — gets an
`mdi-content-copy` icon button that copies the raw value to the clipboard (`navigator.clipboard`,
with a fallback for non-secure contexts / older browsers) and confirms via the existing
`SnackbarContext`. Apply this consistently across `ContactInformation.tsx`'s field rows, not just
the newly-tappable ones — the ask is explicit that *every* field gets it, even ones with no other
action.

### 4. Configurable messaging/social link-type registry

New entity, e.g. `LinkFieldType`, following `circle_controller.go`'s CRUD idiom:

- `ID` (UUID, `BeforeCreate`-generated per this repo's UUID-PK convention), `UserID`, `Name`
  (e.g. "WhatsApp"), `Protocol` (a URI template string with a `{value}` placeholder, e.g.
  `https://wa.me/{value}`), `Category` (`messaging | social | other`), `Icon` (optional MDI slug —
  free text for custom entries, a curated dropdown for defaults), `IsDefault` (bool — seeded vs.
  user-added, so defaults can be told apart from customizations in the UI), `Position` (for
  ordering).
- Full CRUD + a Settings surface to add/edit/delete/reorder — this is the "Contact field types"
  configuration screen the original ask described.
- **Seed defaults per user** (lazily on first fetch if the user has none, following the pattern
  used elsewhere in this repo for per-user default rows) for:
  - **Messaging**: Signal, Messenger, WhatsApp, Telegram, Discord, Wire, Session, Line, Element,
    GroupMe, Slack
  - **Social**: X/Twitter, BlueSky, Instagram, Threads, TikTok, Reddit, Spotify, Twitch
  - Best-effort URI templates for each — some of these (Discord in particular) don't have a
    stable public profile-by-handle URL; seed the best available and leave it user-editable
    rather than blocking on getting every one perfectly right, since the whole point of the
    registry is that a wrong or missing template is fixable without a code change.

### 5. Applying the registry to existing data

`SocialProfiles`, `OtherOnlineServices`, and IMPP entries are already `OnlineService`-shaped
(`Service`, `URI`, `User`, `Contexts`, `Pref`, `Label` — per T29 WP3). Resolve a tappable link as:

1. If `OnlineService.URI` is already a full URL, link to it directly (it's already a complete
   profile link — no template needed).
2. Else, case-insensitively match `OnlineService.Service` against `LinkFieldType.Name`. If
   matched and `OnlineService.User` (handle) is populated, build the link by substituting
   `{value}` in the matched `Protocol` template.
3. Else (no match, no URI), the field is not tappable — display as text with just the copy
   button, same as any other non-actionable field.

### 6. Raw "other links" (`Card.Links`)

These are already full URLs by definition — make them directly tappable as-is, plus the copy
button.

### 7. Address → map link

Build a `geo:` URI when `Address.Coordinates` is populated. Otherwise, fall back to a web map
link built from the formatted address (`https://maps.google.com/?q=<address>`) — a web link
rather than trying to detect Apple vs. Google client-side, since both platforms' map apps
register as handlers for their own web URLs via universal/app links, so this degrades correctly
either way without UA-sniffing. Copy button applies here too (copies the formatted address, not
the URI).

## Traps

- **`Protocol` is a user-editable string used to build an `<a href>`.** Validate it server-side
  with the existing `safeurl` validator (`middleware/validation.go`) so a `javascript:` or other
  unsafe scheme can't be stored, and treat the built-from-template URL the same way client-side
  before rendering as `href` (don't trust that `safeurl` alone makes runtime substitution safe —
  escape/validate the *substituted* value too, not just the template).
- **Icon-only buttons need `aria-label`s** (call, text, copy, etc.) — don't lose accessible names.
- **Soft vs. hard delete (T26).** `LinkFieldType` is user-authored configuration (soft-delete per
  T26's rule), but since a user may plausibly want to re-add a deleted type with the same name,
  give it a **partial unique index** on `(user_id, name) WHERE deleted_at IS NULL` — same pattern
  `idx_contacts_vcard_uid_user` uses — rather than a plain unique index that would block
  re-creation.
- **Cascade delete**: add `LinkFieldType` to `DeleteUser`'s cascade list (it's not
  contact-scoped, so it doesn't belong in `DeleteContact`'s).
- **i18n**: new UI strings (the Settings CRUD screen, button labels/tooltips) need real
  translations in all 5 locales, not just English.
- Don't let the copy-button rollout regress T25/T29's data-preservation work — adding action
  buttons around a field must not change how that field round-trips through `api/contacts.ts`'s
  adapters.

## Done when

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` green, including a real-DB test
  (`database.InitDB`) for `LinkFieldType` CRUD, the unique-index-after-soft-delete case, and
  `DeleteUser` cascade.
- `npx tsc --noEmit` clean, `npx vitest run` green.
- Hand-verified in a real browser: tapping a mobile number's `tel:`/`sms:` buttons and a landline's
  `tel:`-only button behave as specified; a fax number shows copy only; tapping an email opens a
  mail client; a WhatsApp/Signal/etc. handle with no full URI resolves through the registry to a
  working deep link; copy buttons work on both tappable and non-tappable fields; an address with
  no coordinates falls back to a working map search link.
- Settings CRUD screen: add a custom link type, confirm it applies to a matching `OnlineService`
  entry without a code change.
- All 5 locale files have real translations for every new string.

## Landing note

Landed on `feature/T34-contact-field-linking`.

**Backend:** `LinkFieldType` (UUID PK, soft-delete, partial unique index on `(user_id, name)` —
migration `000009`) with full CRUD + a `PUT /link-field-types/reorder` endpoint
(`link_field_type_controller.go`). No existing "seed defaults on first fetch" pattern existed in
this repo to copy (the ticket implied one) — `ListLinkFieldTypes` lazily seeds
`LinkFieldTypeDefaults` (11 messaging + 8 social) on a user's first call, racing concurrent
first-fetches safely via the partial unique index + a recount-and-swallow fallback. Discord/Wire/
Session/GroupMe/Slack seed with an empty `Protocol` (no stable public profile-by-handle URL, per
the ticket's own call) rather than blocking. `DeleteUser` hard-deletes `LinkFieldType` rows
(`Unscoped()`, matching every other `DeletedAt`-bearing entity in that cascade — soft-deleting
there would just leave orphaned tombstones behind for a gone account). `openapi.yaml` + the
route-coverage/schema-spot-check drift tests were updated for the 5 new endpoints.

**Frontend:** `resolveOnlineServiceLink` (client-side, in `utils/linkResolution.ts`) matches
`OnlineService.Service` case-insensitively against the registry and substitutes *every*
occurrence of `{value}` (a `split('{value}').join(...)`, not a single `.replace()`, since a
user-authored template may reference the placeholder more than once) — deliberately kept out of
the backend to avoid widening the contact response. A CopyButton component +
`utils/clipboard.ts` (Clipboard API with an `execCommand` fallback) is on every field row in
`ContactInformation.tsx`, including the single-scalar `EditableField`-based ones (birthday,
gender, organization, how-we-met, ...) via a shared change to `EditableField.tsx` itself, not
just the newly-tappable rows. Single-action fields (email, address, online-service handles, raw
links) became the tappable value itself; phone — the only multi-action field — gets discrete
call/text icon buttons instead, per the ticket's own table, keyed off the phone's full
`features`/`contexts` arrays (not just the first token) so a real-world multi-TYPE vCard import
like `TYPE=VOICE,CELL` still gets its text button. Every built/substituted/passthrough href
(including a full-URI `OnlineService.URI` passthrough) is checked against both
`looksLikeAbsoluteUri` and `isSafeUrlString` (a client-side mirror of the backend's `safeurl`
validator) before use — a value with no dangerous scheme but also no scheme at all (e.g. a bare
handle mistakenly imported into the URI slot) is not tappable either.
`LinkFieldTypesSettings.tsx` (Settings CRUD screen) uses up/down arrow buttons for reordering —
no drag-and-drop library exists in this repo, and one type registry didn't warrant adding one.
The registry fetch inside `ContactInformation.tsx` only fires when `socialProfiles` or
`otherOnlineServices` is actually enabled (`ListLinkFieldTypes` does a write — lazy defaults
seeding — on a user's first call, and neither field is in `DEFAULT_ENABLED_CONTACT_FIELDS`).

**Scope decision — IMPP is not routed through the registry.** The ticket's §5 names
`SocialProfiles`, `OtherOnlineServices`, *and* IMPP as `OnlineService`-shaped. The first two are;
IMPP isn't, on the frontend: `cardImppToValues` (`api/contacts.ts`, pre-existing, T29) flattens
IMPP to the same `ContactValue` (type/value) shape phones and emails use, via `MultiValueField`,
discarding `Service`/`User` entirely — only `URI` survives. `renderUriValueList` links an IMPP
entry directly when its value is itself a safe absolute URI (the existing `xmpp:`/`sip:`-style
case) but cannot registry-match a bare handle, because the handle's originating service name
never reaches the frontend. Making IMPP registry-matchable would mean rebuilding its editor
around `OnlineServiceEditor` instead of `MultiValueField` — a real UI change with its own
review/i18n/test surface, not a tappable-link concern — so it's left as a follow-up rather than
folded into an already-large ticket.

**Reorder requires the complete set.** `ReorderLinkFieldTypes` checks the submitted ID list
against the user's *total* row count, not just that every submitted ID is owned and duplicate-
free — a genuine but incomplete subset is rejected, matching the handler's own doc comment,
`api/linkFieldTypes.ts`'s doc comment, and `openapi.yaml`'s description (all three said this from
the start; the check itself was strengthened to match).

**Tests:** backend real-DB test (`link_field_type_real_db_test.go`) covers CRUD, lazy seeding, a
best-effort forced-concurrency seeding-race test (32 workers behind a start barrier — SQLite's
own write serialization makes the double-INSERT window narrow, so this is best-effort like
`database/concurrent_write_test.go`'s own precedent, not a guaranteed repro), the partial-unique-
index-after-soft-delete case, the duplicate-name 409, `safeurl` rejection, reorder rejecting a
foreign owner's real UUID4 *and* a genuine-but-incomplete subset of the caller's own IDs — every
one of these hand-verified by breaking the corresponding code and confirming the test actually
fails, then restoring. Frontend: `linkResolution.test.ts` (pure resolution logic, including the
scheme-less-URI and repeated-`{value}` cases), `ContactInformation.test.tsx` extensions
(per-phone-type buttons including multi-TYPE-token imports, link hrefs, copy buttons on every
field, a positive registry-resolution case reached through the real hook via a mocked API module
— not just the pure resolver in isolation, a negative "fetch doesn't fire" case, and a round-trip
regression test), `LinkFieldTypesSettings.test.tsx`, `CopyButton.test.tsx`, `clipboard.test.ts`,
`api/linkFieldTypes.test.ts`, plus two new Playwright specs (`linkFieldTypes.spec.ts`,
`contactFieldLinking.spec.ts`) run against the `docker-compose.test.yml` stack. The e2e
social-profile-resolution path is deliberately *not* automated (it's off by default in
`DEFAULT_ENABLED_CONTACT_FIELDS`, and flipping that shared TEST_USER setting mid-suite would race
other spec files under `fullyParallel: true`) — covered by the component-level registry test
instead, plus a manual hand-verification pass confirming a custom link type added via Settings
resolves on a contact's page with no code change.

**All 5 locale files** got real translations for `common.copy(Failed|Value)`, `contactDetail.call`/
`.text`, and the full `settings.linkFieldTypes.*` namespace.

**Reviewed by an Opus subagent** (security, test comprehensiveness, ticket completeness,
regressions, idiom, docs) before landing; every finding above reflects that pass's fixes, not the
first-draft implementation — see this branch's git history for the pre-review/post-review split.
