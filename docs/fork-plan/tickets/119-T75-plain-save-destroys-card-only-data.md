# T75 — A plain `db.Save` on a loaded contact silently destroys all Card-only data ⚠

| | |
|---|---|
| **Platform** | Backend |
| **Rating** | 5 — silent, irreversible data loss on shipped, real-data paths |
| **Size** | M — the mechanism is one function, but the fix is a semantic change to `BeforeSave` plus an audit of every save site |
| **Depends on** | Nothing |
| **Status** | TO BE DONE |
| **Source** | Found during the 2026-08-11 grooming pass while investigating [T67](111-T67-android-address-import-parsing.md)'s "backend drops unmatched address kinds" claim. Not previously reported by testing — it is invisible from the UI until the data is already gone. |

## Why this exists

**This was reproduced against the real migrated schema, not reasoned about.** A throwaway
`database.InitDB` test created a contact through `ApplyRecordToContact`, reloaded it, set one
unrelated flat field, and called `db.Save` — the shape of two shipped handlers. Result:

```
--- after create (ApplyRecordToContact path) ---
  SpeakToAs:    &{Pronouns:[{Pronouns:she/her}]}
  PersonalInfo: [{Kind:hobby Value:sailing}]
  addr component: kind="postOfficeBox" value="PO Box 42"
  addr component: kind="apartment"     value="Apt 3B"
  addr component: kind="floor"         value="4"
  addr component: kind="name"          value="123 Main St"
  ... locality / region / postcode / country ...

--- after plain db.Save (photo set) ---
  SpeakToAs:    <nil>          ← GONE
  PersonalInfo: []             ← GONE
  addr component: kind="name"  value="123 Main St"
  ... locality / region / postcode / country ...
                               ← postOfficeBox / apartment / floor GONE
```

### The mechanism

`Contact.BeforeSave` (`backend/models/contact.go:232-285`) branches on the transient
`cardSetDirectly` marker:

- If `ApplyRecordToContact` ran immediately before the save, `cardSetDirectly` is true and `c.Card`
  is left alone. This is the guard the existing
  `TestApplyRecordToContact_PreservesUnmappedCardData` pins, and it works.
- Otherwise — **including on every contact reloaded from the database**, since `cardSetDirectly` is
  unexported and therefore zero-valued after a GORM load — `BeforeSave` runs
  `record = RecordFromContact(c, DefaultPhotoDir)` and then **overwrites `c.Card` with it**
  (`contact.go:256-259`).

`RecordFromContact` rebuilds a Record from the *flat* columns only. This is exactly the hazard
`/CLAUDE.md` backend trap #3 documents ("`RecordForContact`, not `RecordFromContact` — the latter
rebuilds from flat fields and **silently drops** `SpeakToAs`, `PersonalInfo`, projections"). The
trap note is written as a rule for *call sites*; what it does not say — and what makes this bug
possible — is that **`BeforeSave` itself is one of those call sites**, so the drop happens
underneath any caller that didn't happen to route through `ApplyRecordToContact`.

### What is actually lost

Everything in `Card` with no flat-column home:

- `Card.SpeakToAs` — pronouns and grammatical gender (RFC 9554).
- `Card.PersonalInfo` — hobbies, expertise, interests.
- **Address components outside the flat 6-field projection.** `ContactAddress`
  (`models/contact.go:61-68`) has only `Type/Street/City/Region/Postal/Country`, and
  `contactAddressFromNeutral` (`models/contact_record_reverse.go:283-303`) maps only the five kinds
  that fit. The other twelve RFC 9553 kinds — `postOfficeBox`, `apartment`, `floor`, `room`,
  `building`, `number`, `block`, `district`, `subdistrict`, `direction`, `landmark`, `separator` —
  have no slot and are dropped on the rebuild. These are **not hypothetical**: `vcard4/adapter.go:944,961-965`
  emits `postOfficeBox`/`apartment`/`floor`/`number`/`block`, and `vcard3/adapter.go:688` emits
  `postOfficeBox`, so any VCF import of an apartment or PO-box address produces them.

Anything with a flat home (names, emails, phones, birthday, org) round-trips fine — which is
precisely why this has gone unnoticed: the fields a user looks at after the operation are intact.

### Confirmed live triggers

All three are shipped paths running against real production data:

1. **Profile-photo upload** — `backend/controllers/photo_controller.go:167` and `:184` load the
   contact, set `Photo`/`PhotoThumbnail` (or nothing, in the no-new-photo branch), and call
   `db.Save(&contact)` with no `ApplyRecordToContact`. Setting a contact's photo wipes their
   pronouns, hobbies, and apartment number.
2. **VCF/CSV import merge into an existing contact** — `backend/services/import_session.go:297` and
   `:421` call `MergeImportedContact(&existing, incoming)` (`import_service.go:1127`, which mutates
   flat fields only and never sets `cardSetDirectly`) and then `tx.Save(&existing)`. This is the
   same family as [T49](58-T49-vcf-import-merge-corrupts-existing-contact.md), on a different
   mechanism.
3. **The contact Undo button** ([T60](79-T60-audit-trail-ui.md)) — `undoContact`
   (`controllers/audit_controller.go:100-140`) does
   `RecordFromContact(&before) → ApplyRecordToContact(&current, …)`, where `before` came from
   `json.Unmarshal` of the audit snapshot. **`Contact.Card`/`CRM`/`Passthrough` are all
   `json:"-"` (`models/contact.go:140-142`)**, so the snapshot never contained them and `before.Card`
   is the zero value. The rebuilt Record therefore carries only what the flat fields can express, and
   `ApplyRecordToContact` overwrites `current.Card` with it — additionally setting `cardSetDirectly`,
   so `BeforeSave` won't second-guess it.

   This one is not a missed call site; it is a *deliberate decision made without full information*.
   The function's own doc comment says the columns are "rebuilt from the restored flat state rather
   than diverging" — sound intent, except the flat state cannot represent `SpeakToAs`,
   `PersonalInfo`, unprojected address components, or `CRMEnvelope.Kind` (T27's pet/animal marker,
   which `TestApplyRecordToContact_PreservesCRMKind` notes lives only in the `crm` JSON column). So
   "rather than diverging" means, in practice, "by deleting whatever flat can't hold."

   **Undoing a contact edit destroys data, and undo is the one button in the product whose entire
   purpose is recovery.** It also silently reverts a pet contact to `individual`.

Audited and **safe** (they route through `ApplyRecordToContact` first, several citing the traps
explicitly): `contact_controller.go:487` (`UpdateContact`), `services/wedding_sync.go:118`,
`services/contact_sync_service.go:443`. `services/immich_service.go:442` saves an
`ExternalActivity`, not a `Contact`.

### Already-lost data is not recoverable

Checked, rather than assumed: the audit trail would be the obvious recovery source, since
`auditBeforeSave` (`models/audit.go:154-168`) re-reads the row and stores `redactedJSON(&old)`. But
that is `json.Marshal`, and `Card`/`CRM`/`Passthrough` are `json:"-"` — **the audit trail has never
captured any nested contact data.** There is no snapshot to restore from, for this or for anything
else. Say so plainly in the landing note rather than implying the fix recovers anything.

That gap is also precisely why trigger 3 exists, so the two are fixed together or not at all — see
the scoping note below.

## What to build

**Fix the mechanism, not the call sites.** Making each of the four offending saves call
`ApplyRecordToContact` would work today and rot immediately: the failure is silent, so every future
`db.Save(&contact)` is a fresh chance to reintroduce it with no test or compiler to catch it. This
is the same reasoning `/CLAUDE.md` uses to reject operation-based variance in delete semantics.

1. **Change `BeforeSave`'s non-`cardSetDirectly` branch from replace to merge.** `c.Card` is already
   populated with the persisted Card when the row was loaded, so the information needed is in hand —
   the bug is that a wholesale overwrite throws it away. Derive the flat-owned sub-structures from
   the flat fields as today, but write them *onto* the existing `c.Card` rather than replacing it,
   leaving Card-only members (`SpeakToAs`, `PersonalInfo`, and each `Address`'s non-projected
   components) untouched.
2. **Addresses: decide by dirty-comparison, not by positional pairing.** The first draft of this
   ticket called the merge rule "the part most likely to be wrong" and worried about flat lists that
   are deleted, reordered, or differ in length. **Checking which code actually writes the flat arrays
   collapses that problem**: on every plain-save path, nothing does.
   `MergeImportedContact` (`import_service.go:1127+`) mutates only flat *scalars* — `Address`,
   `Email`, `Phone`, names, org — and never `Addresses`/`Emails`/`Phones`. `photo_controller` touches
   only `Photo`/`PhotoThumbnail`. The only writers to the flat arrays are
   `contact_merge_service.go:102-104` (manual merge) and `contact_record_reverse.go` (which *is*
   `ApplyRecordToContact`'s own machinery, i.e. the `cardSetDirectly` path).

   So the rule can be simple and exact. Compute the flat projection of the loaded `c.Card`; compare
   it against `c.Addresses`:
   - **Equal** → the caller never touched the flat array, so keep `Card.Addresses` untouched,
     unprojected components and all. This is every live trigger today.
   - **Different** → the caller deliberately expressed intent through the lossy shape, so honor it
     and rebuild that array from flat. Losing unprojected components there is correct: the caller
     said so.

   No positional pairing, no reordering rules, no length heuristics.
3. **Card members with no flat representation are preserved unconditionally.** `SpeakToAs`,
   `PersonalInfo`, and `CRMEnvelope.Kind` cannot be "edited via flat fields" because they have no
   flat field — so there is nothing to compare and no ambiguity at all. Carry them over from the
   loaded Card, always. This is the bulk of the bug and it carries zero design risk; it can land
   ahead of item 2 if the change is being staged.
3. **Regression-test each lost category separately** against a `database.InitDB` schema, not
   `AutoMigrate` (trap #1): create via `ApplyRecordToContact` → reload → mutate one flat field →
   `db.Save` → reload → assert `SpeakToAs`, `PersonalInfo`, and `postOfficeBox`/`apartment`/`floor`
   all survive. Hand-verify per `/CLAUDE.md`: these tests must fail against today's `BeforeSave`.
   The reproduction above is a ready-made starting point.
4. **Add a test at each confirmed trigger** — one that uploads a photo, one that runs an import
   merge, one that undoes a contact edit — so the guarantee is pinned at the handler level too, not
   only in `models`.
5. **Stopgap for trigger 3 (undo).** `undoContact` (`controllers/audit_controller.go:100-140`) must
   stop overwriting `current.Card` with a Record rebuilt from a snapshot that never contained one.
   Apply the same rule as item 3: restore the flat state from the snapshot, but **carry
   `current.Card`'s unprojected members through** — `SpeakToAs`, `PersonalInfo`, per-address
   components outside the flat five, and `CRMEnvelope.Kind`.

   The result is honest rather than complete: undo reverts everything the snapshot actually recorded
   and leaves everything else as it is. That is strictly better than today, where it deletes what it
   cannot restore. **Say so in the UI copy or the ticket's landing note** — a user pressing Undo
   should not believe a pronoun change was reverted when it wasn't.
   [T82](126-T82-audit-snapshots-miss-nested-contact-data.md) closes the remaining gap.

## Traps

- **`cardSetDirectly` is one-shot and unexported.** It is cleared inside `BeforeSave`
  (`contact.go:254`) and is always false on a freshly loaded row. Any fix that leans on it holding a
  value across a load/save cycle is wrong.
- **Don't "fix" this by widening `ContactAddress`.** Adding apartment/PO-box fields to the flat
  struct changes the `MultiValueField`/`AddressFields` frontend editing contract (`/CLAUDE.md`,
  frontend conventions) and is a separate, larger decision. The flat projection is *allowed* to be
  lossy; what is not allowed is the lossy projection overwriting the authoritative nested one.
- **Existing production data already damaged by this is not recoverable by the fix.** Any contact
  whose photo was set, or which was merged during an import, since these paths shipped has already
  lost this data. Say so in the landing note rather than implying the fix restores anything.

## Scoping — decided 2026-08-11: two tickets

Triggers 1 and 2 are fixed entirely by the `BeforeSave` change above. **Trigger 3 is not**, and can't
be: `undoContact` deliberately bypasses `BeforeSave` (it sets `cardSetDirectly`), so the only real
fix is giving undo something to restore — i.e. making the audit snapshot carry
`Card`/`CRM`/`Passthrough`. That is its own body of work with its own decisions (snapshot size on the
most-updated entity in the product, redaction over nested data, sensitivity), and it can only ever
help events written *after* it lands.

So: **this ticket stops all three bleeds; [T82](126-T82-audit-snapshots-miss-nested-contact-data.md)
makes undo full-fidelity afterwards.** The stopgap for trigger 3 (item 5 below) is nearly free once
item 3's preserve-what-flat-can't-express logic exists — it is the same rule applied at a second call
site.

The alternative considered was a single ticket doing both, so that "contacts don't lose nested data"
became true end-to-end in one landing. Rejected because it delays an active data-loss fix behind a
storage-cost decision that wants measurement against the production database.

## Related, deliberately not in scope

`AddressComponent.Kind` is not validated anywhere on write, so a client can POST a component with a
kind outside the RFC 9553 registry (`contactmodel/model.go:167-176`) and get a `201` with the value
silently discarded from every flat surface. That is what makes [T67](111-T67-android-address-import-parsing.md)'s
Android bug invisible from the API's side. Worth its own small validation ticket; it is a different
mechanism from this one and fixing it here would muddy both.

## Done when

- A contact carrying `SpeakToAs`, `PersonalInfo`, and an address with `postOfficeBox`/`apartment`/
  `floor` survives a plain `db.Save` with all of it intact.
- All three confirmed triggers (profile-photo upload, import merge into an existing contact, contact
  Undo) have handler-level regression tests.
- Undo no longer clears nested data it cannot restore, and the partial-restore behavior is stated
  somewhere a user will see it.
- All new tests were hand-verified to fail against the pre-fix `BeforeSave`.
- The address-merge rule chosen in item 2 is written down in `BeforeSave`'s doc comment.
- `cd backend && go build ./... && go vet ./... && gofmt -l . && go test ./...` green.
