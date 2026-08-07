# T58 — No UI to see or set "preferred" on phone/email (and URL/IMPP)

| | |
|---|---|
| **Rating** | 2 (per user, 2026-08-06) |
| **Size** | S |
| **Depends on** | — |
| **Alpha** | n/a — real data exists; this only adds a UI affordance for a field that already persists, no schema change |
| **Source** | User question during T50 review, 2026-08-06: "do we have a concept of preferred for phone/email? I don't recall seeing that in the UI." Confirmed: we do, but it's invisible. |

## Why this exists

The neutral vCard model (`contactmodel.Email`/`Phone`/`URL`(`Resource`)/`OnlineService`, all in
`backend/contactmodel/model.go`) carries a `Pref *int` field mapping to vCard's `PREF` parameter.
`contactmodel/projection.go`'s `DeriveProjection` sorts each family by `(Pref asc, index)` to pick
`PrimaryEmail`/`PrimaryPhone`, and `Contact.BeforeSave` (`backend/models/contact.go:232`) uses that
projection to set the flat `Contact.Email`/`Contact.Phone` scalars — the values shown in contact
list rows, search results, and anywhere else the app shows "the" email/phone for a contact rather
than the full list. So **an imported vCard's `PREF` already determines which entry the app treats
as primary today** (T50 just fixed the vCard 2.1 path that feeds this).

The frontend's `ContactValue`/`ContactAddress` types (`frontend/src/api/contacts.ts`) already carry
`pref` too, explicitly as round-trip passthrough (see the comment at `contacts.ts:7`) — `cardEmailsToValues`/
`valuesToCardEmails` and their Phone/URL/IMPP siblings read and re-emit it faithfully. **The data is
never lost.** But nothing in the UI reads it:

- `MultiValueField.tsx` (`frontend/src/components/MultiValueField.tsx`) — the actual editor for
  Emails/Phones/URLs/IMPPs on the contact form — only renders a Type dropdown and a Value field per
  row. No star, no checkbox, no way to see or set which entry is preferred.
- `ContactInformation.tsx`'s read-view renderers (`renderPhoneList`, `renderEmailList`,
  `renderUriValueList`, all in `frontend/src/components/ContactInformation.tsx` around line 195-270)
  render every row identically — no visual distinction for whichever one has `pref === 1`.

Net effect: the preference set by an import survives, invisibly, and a user manually adding
phones/emails has no way to express one at all. If someone reorders/edits an entry expecting to
change which one is "the" phone shown elsewhere, nothing they do in the visible UI has any effect —
the actual selection is driven by data they can't see. That's a real "looks lost" bug even though
nothing is actually lost, per the user's framing.

## What to build

1. **`MultiValueField.tsx`**: add a per-row preferred toggle (a star `IconButton`, filled when
   `row.pref === 1`, outline otherwise). Clicking sets that row's `pref` to `1` and clears `pref`
   (to `undefined`, not `0` — see Traps) on every other row in the same array, in the local
   `onChange` draft — a single preferred entry per field family, not a ranked list.
2. **`ContactInformation.tsx`**: `renderPhoneList`/`renderEmailList`/`renderUriValueList` show a
   small, non-interactive star/badge next to whichever row has `pref === 1`, so the preferred entry
   is visible outside edit mode too (this is where a user would actually notice "this is the one
   used elsewhere").
3. New locale strings (tooltip/aria-label for the toggle, e.g. "Preferred" / "Set as preferred") —
   real translations in all five locale files (`en`, `de`, `es`, `fr`, `it`), not English
   placeholders (`src/i18n/locales.test.ts` enforces this).
4. No backend change: `ContactRecordInput.Card` (`backend/models/contact_summary.go:130`) binds
   directly to `contactmodel.Card`, whose `Email`/`Phone`/`Resource`/`OnlineService` types already
   have `Pref *int json:"pref,omitempty"` — a `pref` value sent from the frontend already lands
   correctly today. Confirmed by reading the actual bind path, not assumed.

Scope is `MultiValueField`'s four families (Emails/Phones/URLs/IMPPs) — same shared component,
so URL/IMPP get the toggle "for free" alongside phone/email. **Addresses are explicitly out of
scope**: `ContactAddress` also carries a `pref` passthrough field, but its editor (`AddressFields`,
a separate component per `/CLAUDE.md`'s "MultiValueField/AddressFields' editing contract") isn't
touched here — a natural follow-up ticket, not bundled into this one, to keep this one small and
scoped to what was actually asked about.

## Traps

- **Binary preferred, not a 1–100 ranking.** RFC 6350's `PREF` parameter technically supports a
  full 1-100 ranking, and `vcard4.Adapter` round-trips arbitrary integers — but `vcard3.Adapter`'s
  `isPreferred()` (`vcard3/adapter.go:884`) only ever checks `pref == 1` (vCard 3.0/2.1's grammar has
  no ranking, just a `PREF` TYPE token), and that's also all `DeriveProjection` needs to pick a
  primary. Building a full ranking UI would be scope creep for a capability the format doesn't
  really support end-to-end. One star per family, `pref: 1` on the chosen row, `undefined` on the
  rest.
- **Clear with `undefined`, not `0`.** `0` is not a valid `PREF` value (RFC 6350 §6.2.2's Kv range
  starts at 1) and `effectivePref()` (`contactmodel/projection.go:97`) treats *unset* Pref as "no
  explicit preference, ranks last" — sending `0` would be a real, different, non-vCard-legal value,
  not "no preference."
- **Update the draft locally, not just on save.** `MultiValueField`'s existing rows are edited via
  `onChange`/`updateRow` against the in-memory draft (same pattern as add/remove) — the previous
  preferred row must be un-marked in that same `onChange` call, not left stale until a re-fetch.
- **All five locale files need real translations**, not just `en` — `src/i18n/locales.test.ts` will
  fail the build otherwise (`/CLAUDE.md` frontend trap #5).
- **vitest has no auto-cleanup here** — any new component test needs an explicit
  `afterEach(cleanup)` (`/CLAUDE.md` frontend trap #1).
- Don't couple this to array order/reordering — this is an explicit independent signal, not a
  drag-to-reorder feature. No reordering UI is in scope.

## Done when

- `cd frontend && npx tsc --noEmit && npx vitest run` green.
- Marking a phone/email row preferred in the edit form, saving, and reloading the contact shows the
  same row still marked preferred (proves the new UI and the pre-existing passthrough plumbing
  actually agree, not just that the toggle renders).
- Marking a new row preferred visibly un-marks the previously preferred row in the same list, both
  before and after save.
- The read-only contact detail view (not just the edit form) visibly distinguishes the preferred
  entry.
- All five locale files carry real, non-English translations for the new strings.
- Hand-verified end to end: import a vCard with an explicit preferred email (a plain
  `EMAIL;TYPE=home;PREF=1:...` 4.0 property, or 2.1's bare `EMAIL;PREF;HOME:` shape T50 fixed) and
  confirm the star lands on the correct entry with no manual edit — proving the import path and this
  new UI actually agree, not just that each was tested in isolation.
