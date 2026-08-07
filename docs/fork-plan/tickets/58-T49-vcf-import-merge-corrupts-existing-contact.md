# T49 — VCF/CSV import "merge with existing contact" silently corrupts and orphans real data

| | |
|---|---|
| **Rating** | 5 — active, proven data-loss risk on real production data, not a capability gap. "Breaking data is a different, higher bar" (`/CLAUDE.md`) is exactly what this violates today. |
| **Size** | M |
| **Depends on** | — |
| **Alpha** | Data-safety critical. Real production data exists; this bug is reachable through completely normal usage (import a vCard/CSV row that name-matches an existing contact) and has been **reproduced against a real migrated database**, not inferred. |
| **Source** | v0.3.0 post-release testing, 2026-08-06 — user reported "importing to merge with an existing contact erased all fields rather than appending... gifts was also wiped out" |

## Why this exists — reproduced, not theorized

Reproduced end to end with a real Go test against a real `database.InitDB`-migrated SQLite file
(scratch test, not committed — this ticket is the record): created a contact with a real phone,
email and a linked `Gift` row; ran it through the exact sequence `ConfirmVCF`'s `"update"` branch
uses (`services/import_session.go:418-420`) — `MergeImportedContact(&existing, incoming)` then
`tx.Save(&existing)`. Result, straight from the test output:

```
existing contact created: VCardUID=74f397f9-... Phones=[{ +1-608-000-0000}] Emails=[{ elizabeth@example.com}]
incoming (parsed from a real-world vCard): Phones=[{ }] Emails=[{ }] VCardUID=freshly-generated-uuid
AFTER merge+save: VCardUID=freshly-generated-uuid (was 74f397f9-...) Phones=[{ }] Emails=[{ }]
gifts still linked to the ORIGINAL VCardUID (74f397f9-...): 1
```

Two independent, compounding bugs, both inside `MergeImportedContact`
(`services/import_service.go:1016-1106`) — which is shared by **both** VCF and CSV import (see
`import_session.go:290,293,418` — a CSV-import fix here is not optional, it's the same function):

**1. Multi-valued fields are replaced, not merged, whenever the incoming side has *any* entry —
even an empty one.** `if len(incoming.Phones) > 0 { existing.Phones = incoming.Phones }`
(`import_service.go:1064-1066`, same pattern for `Emails`/`Addresses`/`URLs`/`IMPPs`) checks
*length*, not *content*. An adapter that produces one entry with a blank value — which the vCard 2.1
parsing gap in [T50](59-T50-vcard21-import-blank-fields.md) demonstrates is a real, live case —
satisfies `len() > 0` and wholesale replaces the existing (real, populated) array with the garbage
one. Even setting T50 aside, this is *also* wrong for a well-formed vCard: a legitimate re-import
that doesn't happen to carry every phone/email the existing contact already has will still delete
the ones it doesn't mention. "Merge" here means "last import wins," not "combine."

**2. `existing.VCardUID` gets silently reassigned on every update.**
`if incoming.VCardUID != "" { existing.VCardUID = incoming.VCardUID }` (`import_service.go:1103-1105`).
`ParseVCF` (`import_service.go:203-205`) mints a **fresh random UUID** for any source vCard lacking
a `UID:` property — which describes most vCards not round-tripped through this app's own CardDAV
sync (Google/Apple/Android exports routinely omit it, confirmed on the real file this session
tested against). So on the overwhelming majority of real-world "update" imports, the existing
contact's stable identity changes. `VCardUID` is the `entity_id` every graph-adjacent table keys
on — Gifts, LifeEvents, Preferences, RelationshipEdges, CadencePolicy, ConversationAgenda,
FieldValue, HouseholdMember (per `/CLAUDE.md`'s own list of "WP-80+ graph entities"). None of those
rows get deleted — the reproduction proves the `Gift` row survives untouched — but the contact no
longer points at the `entity_id` they're filed under, so every query scoped by
`WHERE entity_id = contact.vcard_uid` returns nothing for them. That is exactly what "gifts was
also wiped out" looks like from the outside: silent orphaning, not deletion, which makes it *harder*
to notice and *harder* to recover from than an outright delete would be.

## What to build

1. **Stop overwriting `existing.VCardUID` on merge, ever.** An existing contact's identity is not a
   field an import should be allowed to change. Drop the `if incoming.VCardUID != "" { ... }` block
   from `MergeImportedContact` entirely — the existing row's `VCardUID` was assigned at its own
   creation and must survive every subsequent merge.
2. **Change the multi-valued-field merge semantics from replace-if-nonempty to genuinely
   append/merge**, filtering out any incoming entry whose value is blank before deciding whether
   there's anything worth merging at all. At minimum: `len()` alone must never be the trigger — check
   whether the incoming entries actually carry content. Decide deliberately (and document the
   decision in this ticket's landing note) whether the merged result is "existing entries plus any
   genuinely new incoming ones" (additive, matches the user's stated expectation) or something else
   — but "replace" is off the table.
3. **Extend `CreateMergeNote`'s coverage** (`import_service.go:1109` on, called just before the
   merge at `import_session.go:414`) so the timeline note it leaves actually reflects what changed
   under the new semantics — a note is only useful if it says what the merge actually did.
4. **Add a real-DB regression test** modeled directly on the reproduction above: existing contact
   with populated Phones/Emails/a linked Gift; incoming contact from an import with a fresh
   VCardUID and either no phone/email or a blank-valued one; assert after `MergeImportedContact` +
   save that `VCardUID` is unchanged, the existing phone/email survive, and the gift is still
   reachable via `WHERE entity_id = contact.vcard_uid`.

## Traps

- This is the exact class of bug `/CLAUDE.md` already names three times over (`Card`/`CRM` direct
  mutation before save, `RecordFromContact` vs `RecordForContact`) — a fourth site hitting a
  documented trap is worth flagging to whoever picks this up: search for other places that mutate
  flat `Contact` fields directly and then call `db.Save`/`db.Updates` without going through
  `ApplyRecordToContact`, since this bug means that audit was incomplete.
- `MergeImportedContact` is shared by CSV import too (`import_session.go:290,293`) — a fix scoped
  only to the VCF code path leaves CSV-import-to-merge exploitable the same way. Fix the shared
  function once; verify both call sites.
- Don't fix this by making `MergeImportedContact` smarter about *this specific vCard's* blank
  fields — fix the general "length isn't content" and "identity isn't mergeable" problems. A
  narrower fix would leave the next malformed-input case exploitable the same way.
- `DetectDuplicate` (`import_service.go:709`) matches on name+email+phone; when email/phone come
  back blank from a bad parse (T50), it can only match by name — worth a cross-reference note in
  T50, not a fix here, but don't assume duplicate-detection quality is this ticket's problem to solve.

## Done when

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` green.
- The real-DB regression test above passes, and was hand-verified to fail against the current code
  first (this ticket's own reproduction already proves it will).
- A second real-DB test proves CSV import's merge path (not just VCF's) is fixed too.
- Hand-verified: import a vCard/CSV row that merges into an existing contact with existing
  phone/email/gifts, using both a well-formed vCard and one with gaps; confirm nothing existing is
  lost and the contact's `VCardUID` is unchanged before and after.
- `CreateMergeNote`'s timeline note is checked to genuinely describe the resulting merge.

## Landing note — 2026-08-06

Both bugs fixed in `services/import_service.go`'s `MergeImportedContact`:

1. **`VCardUID` is no longer touched by merge, at all** — the `if incoming.VCardUID != "" { ... }`
   block is gone. The existing contact's identity survives every merge unconditionally.
2. **Multi-valued fields (`Emails`/`Phones`/`Addresses`/`URLs`/`IMPPs`) now merge additively**, via
   a new generic `mergeContactValues[T]` helper: every existing entry survives unconditionally, and
   an incoming entry is appended only if it has actual content (blank-value entries — the T50
   case — are filtered) *and* isn't a duplicate of something already present (deduped by
   normalized value, so a re-import of the same vCard doesn't pile up copies). Decision per item 2:
   went with "existing entries plus any genuinely new incoming ones" (additive), matching the
   user's stated expectation ("erased all fields rather than appending"). `Circles` was left alone
   — it isn't a vCard multi-valued field in this sense, and `ParseCircles` already filters blanks
   before `Circles` is ever populated, so it was never exposed to this bug.

`CreateMergeNote` was changed to take the incoming `*models.Contact` directly instead of the
flattened `map[string]interface{}` preview — it needs the real `Emails`/`Phones`/etc. slices to
report what an additive merge actually added, which a flattened string map can't carry. Scalar
fields are still reported as `old → new`; multi-valued fields are now reported as `added: x, y`
(and the old per-field `Email`/`Phone`/`Address` *scalar* lines were dropped from the note, since
those scalars are just `Contact.BeforeSave`'s denormalized first-entry projection of the arrays —
reporting both would double up). Both call sites in `import_session.go` (`Confirm`'s shared
update branch and `ConfirmVCF`'s update branch) updated accordingly; `preview.ParsedContact` is
untouched and still serves its original purpose (the API preview table).

Verified: `go build ./... && go vet ./... && gofmt -l . && go test ./...` green. Two new real-DB
tests (`services/import_service_merge_real_db_test.go`, against `database.InitDB`, not
`AutoMigrate`) reproduce the ticket's exact scenario for both VCF and CSV — existing contact with
phone/email/a linked `Gift`, incoming with a fresh `VCardUID` and a non-overlapping/blank-valued
field — and assert `VCardUID` is unchanged, existing data survives, and the `Gift` stays reachable
via `WHERE entity_id = contact.vcard_uid`. Hand-verified per `/CLAUDE.md`'s testing rule: temporarily
reverted both fixes, confirmed all new tests fail (including the two real-DB ones, reproducing the
exact "gift becomes unreachable" symptom), then restored — diffed byte-identical to the fix.
`import_service_merge_test.go`'s pre-existing tests, which had pinned the old replace-semantics as
if it were the intended policy, were rewritten to assert the additive/dedup/blank-filtering
behavior instead (that assertion *was* the bug this ticket fixes).
