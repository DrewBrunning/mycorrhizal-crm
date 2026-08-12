# T82 — Audit snapshots never capture Card/CRM/Passthrough, so undo can't be full-fidelity

| | |
|---|---|
| **Platform** | Backend |
| **Rating** | 3 — a fidelity gap in two shipped features (T18 history, T60 undo), not a bleed |
| **Size** | M — the capture change is small; the storage-cost and sensitivity decisions around it are the work |
| **Depends on** | [T75](119-T75-plain-save-destroys-card-only-data.md) — its stopgap makes undo *non-destructive*; this ticket is what makes it *complete*. Land T75 first. |
| **Status** | **DONE (2026-08-12 — see the landing note at the bottom)** |
| **Source** | Found investigating [T75](119-T75-plain-save-destroys-card-only-data.md), 2026-08-11. Split from it deliberately (option (b) of that ticket's scoping note) so the data-loss fix could ship without waiting on this. |

## Why this exists

`auditBeforeSave` (`models/audit.go:154-168`) captures the pre-update state with
`redactedJSON(&old)`, which is `json.Marshal` plus a deny-list pass. But on `Contact`:

```go
// models/contact.go:140-142
Card        contactmodel.Card        `gorm:"column:card;type:text;serializer:json" json:"-"`
CRM         contactmodel.CRMEnvelope `gorm:"column:crm;type:text;serializer:json" json:"-"`
Passthrough contactmodel.Passthrough `gorm:"column:passthrough;type:text;serializer:json" json:"-"`
```

All three are `json:"-"`. **The audit trail has never captured any nested contact data**, on any
event, since T18 shipped. Two consequences, both live:

1. **[T60](79-T60-audit-trail-ui.md)'s undo cannot restore nested data.** `undoContact` rebuilds from
   the snapshot, so anything only Card can express — `SpeakToAs` (pronouns), `PersonalInfo`
   (hobbies/expertise), address components outside the flat five, `CRMEnvelope.Kind` (T27's
   pet/animal marker) — is simply not in the payload. Before [T75](119-T75-plain-save-destroys-card-only-data.md)
   this made undo actively *destructive*; T75's stopgap makes it preserve what it can't restore, so
   after that lands, undo is safe but silently partial.
2. **The audit history is blind to those fields.** Editing a contact's pronouns produces an audit
   event whose before-snapshot shows nothing changed. The `/audit` page renders an event with no
   visible difference, which reads as a bug in the UI rather than a gap in the data.

**The `json:"-"` tags are correct and must stay.** They exist so the REST API serves the nested model
through `ContactRecordResponse` rather than leaking the storage shape — see `/CLAUDE.md`'s frontend
conventions. The fix is to capture the snapshot from something other than the API-facing JSON tags,
not to change the tags.

## What to build

1. **Capture Card/CRM/Passthrough into the before-snapshot** without altering the model's JSON tags —
   e.g. marshal from a purpose-built snapshot struct, or merge the three serialized columns into the
   redacted map explicitly, rather than relying on `json.Marshal(&contact)`.
2. **Redaction already recurses**, which is the good news: `redactValue` (`audit.go:214-229`) walks
   maps and slices applying a key deny-list, so nested Card objects are covered automatically once
   they're present. **Review `auditDenyList` for Card-shaped keys** anyway — it was written against
   flat field names and nothing has ever exercised it against nested contact data.
3. **Decide the storage cost.** `card` is the largest column on the row, and a before-snapshot is
   written on every update. Snapshotting it multiplies `audit_events` growth for the most frequently
   updated entity in the product. Options to weigh: store it always and lean on
   `AUDIT_RETENTION_DAYS` (default 90) to bound the table; store a diff rather than a full snapshot;
   or store Card only when it actually changed. **Measure before choosing** — take a real contact's
   `card` column length from the production database and multiply by a realistic edit rate, rather
   than estimating.
4. **Decide how sensitivity interacts.** Fields above `normal` (`private`/`secret`, `91.13`) are
   excluded from exports and external sync *in the query*. An audit snapshot is neither, and it stays
   readable through `/audit` after the field itself is deleted — so a secret value would outlive its
   own deletion by up to the retention window. This is the user's own data, so it is not a
   cross-user leak, but it deserves a deliberate call rather than falling out of the implementation.
5. **Handle both snapshot shapes in `undoContact`.** Every event written before this change lacks the
   nested data permanently. Undo must keep T75's preserve-what-isn't-in-the-snapshot behavior for old
   events while doing a full restore for new ones — and must not mistake "absent because old" for
   "absent because cleared."
6. Tests against a `database.InitDB` schema (trap #1): editing a contact's pronouns produces an event
   whose before-snapshot contains them; undoing that event restores them; undoing a *pre-change*
   event still leaves them intact rather than wiping them. Hand-verify per `/CLAUDE.md`.

## Traps

- **Don't remove the `json:"-"` tags to make this easy.** That would change every contact API
  response shape and re-expose the storage model the nested `ContactRecordResponse` exists to hide.
- **Item 5 is the subtle one.** A naive "restore whatever the snapshot has" reintroduces T75's
  destruction for every old event still inside the retention window.
- **Redaction failures are silent by design** — `redactedJSON` returns `""` on a marshal error and
  the event is still recorded without a snapshot (`audit.go:231-233`). A snapshot struct that fails
  to marshal would therefore turn every audit event into an empty one, with no error anywhere. Test
  the failure path.

## Done when

- A contact update that touches only nested data (pronouns, personal info, an apartment number,
  pet/animal kind) produces an audit event whose before-snapshot contains it.
- Undoing such an event restores it.
- Undoing an event written *before* this change leaves nested data intact rather than clearing it.
- The storage-cost and sensitivity decisions from items 3 and 4 are written into the ticket's landing
  note with the measurement that informed them.
- `cd backend && go build ./... && go vet ./... && gofmt -l . && go test ./...` green.

## Landing note — 2026-08-12

**Capture.** `auditBeforeSave`/`auditAfterDelete` now honor an `auditSnapshotProvider` interface
(`models/audit.go`): an entity can supply its own snapshot shape instead of `json.Marshal(&model)`.
`Contact` implements it via a purpose-built `ContactAuditSnapshot` (embedded `Contact` for the flat
fields at top level, plus explicit `card`/`crm`/`passthrough` members — the `json:"-"` tags are
untouched, per the ticket's first trap). Every contact update **and delete** event now carries the
nested columns, so the audit history is no longer blind to pronoun edits and undo can be full-fidelity.

**Undo handles both snapshot shapes.** `undoContact` unmarshals into `ContactAuditSnapshot` and
branches on `HasNested()`: a T82 event (nested columns present — a live contact's Card is never empty,
so presence is unambiguous) gets a full restore via the new `Contact.RestoreFullStateFrom` (flat fields
+ exact before Card/CRM/Passthrough, `cardSetDirectly` so `BeforeSave` keeps them verbatim); a pre-T82
event (keys absent forever) keeps T75's preserve-what-isn't-in-the-snapshot behavior. `HasNested` is
exactly the "absent because old" vs "cleared" discriminator item 5 asked for. Hand-verified both ways:
the pre-T82 guard test (`TestUndoAuditEvent_PreT82SnapshotPreservesNestedData`) fails if undo blindly
full-restores (it wipes the card), and the full-fidelity test (`TestUndoAuditEvent_FullFidelityRestoresNestedData`)
fails if the snapshot doesn't carry the nested data.

**Storage decision — item 3, with the measurement.** Measured against the real migrated schema: a rich
contact's `card` column is **575 bytes** without a photo and **86 KB** with a 400×400 photo (the base64
full-resolution JPEG lives in `Card.Media`; `PhotoThumbnail` is the 48×48 fallback). The photo is the
whole storage problem, and it is **flat-owned** — `Contact.Photo`/`PhotoThumbnail` and the on-disk file,
already carried by the flat half of the snapshot. So the chosen rule: **snapshot the full nested
Card/CRM/Passthrough always, but strip `Card.Media` photo entries first.** Nested data with no flat
home (pronouns, PersonalInfo, unprojected address components, rich per-entry metadata, `CRMEnvelope.Kind`,
Passthrough) is captured completely; the 86 KB→575 B (≈150×) dominant cost is removed; non-photo media
(logo/sound, which have no flat home) stay. Undo re-derives the photo from the restored `Photo` path
(`RestoreFullStateFrom` → `RecordFromContact` + `mergeMedia`), so the restored card stays consistent for
CardDAV/REST readers. Table growth is bounded by `AUDIT_RETENTION_DAYS` (default 90); the `/audit` list
response (100 events, snapshots included) stays in the tens-of-KB range rather than the MBs that photo
embedding would produce. "Store only when changed" was rejected: nearly every real edit changes the card
(a name edit rewrites `Card.Name`), so it would rarely skip the payload while adding an
"empty means unchanged" ambiguity. A diff was rejected as over-complex for a single-operator log.

**Sensitivity decision — item 4.** Include it. The audit snapshot is neither an export nor an external
sync surface — it is the user's own secondary record of their own data, readable only by that user
through `/audit`. The `private`/`secret` exclusion is a read-time export rule
(`RecordForContactFiltered` projecting relationship edges/preferences/custom fields); it does not apply
to the stored card. Excluding sensitive content from snapshots would break exactly the undo this ticket
exists to provide (a user who deletes a secret note and undoes should get it back). Accepted property:
a deleted value stays recoverable from the snapshot for up to the retention window — that is what an
audit trail is for, and it is bounded.

**Deny-list review — item 2.** `redactValue` already recurses into maps and slices, so the nested
columns are redacted automatically once present. `auditDenyList` was reviewed against Card-shaped keys:
no `contactmodel` field name collides (the closest, `Author`, ≠ `authorization`; the deny-list matches
whole key names with underscores stripped). No additions needed. A nested import can still carry a
deny-listed *property name* (e.g. an `X-PASSWORD` vCard prop, whose `Name` key holds the literal
"password") and that is stripped as a value check would want. Pinned by
`TestAudit_SnapshotRedactionReachesNestedData`.

**Known limits, stated plainly.** (1) `PhotoThumbnail` remains `json:"-"` and un-snapshotted (pre-existing
T75 behavior, unchanged here), so undoing a photo *upload* restores the `Photo` path and re-derives the
card photo from it but leaves the thumbnail as-is. (2) Events written before this change permanently lack
nested data — `HasNested()` routes them to preserve-not-restore, and nothing can recover what was never
captured. (3) Snapshots for contacts with rich cards are now a few hundred bytes larger per event.

**Tests** (all hand-verified to fail against the pre-fix code per `/CLAUDE.md`): model-level
`contact_audit_snapshot_test.go` (nested data captured on update and delete; photo stripping with
non-photo preserved; redaction reaches nested depth; snapshot round-trip; photo re-derivation on restore),
and controller-level `audit_controller_t82_test.go` (full-fidelity undo of a pronouns-only edit; the
pre-T82 preserve path). Existing audit/undo tests unchanged and green.
