# T107 — Merge permanently destroys the loser's attachments, preferences, cadence policy and external links ⚠

| | |
|---|---|
| **Platform** | Backend |
| **Rating** | 5 — silent, unrecoverable data loss on real production data |
| **Size** | M |
| **Depends on** | Nothing. **Should land before** [T92](136-T92-bulk-merge-from-contacts-list.md), which makes merge much easier to reach. |
| **Status** | **DONE** (2026-08-13) |
| **Source** | Not reported. Found while investigating [T95](139-T95-merge-keeper-shows-stale-circles.md) during the 2026-08-13 beta triage. |

## Why this exists

`CommitContactMerge` (`backend/controllers/contact_merge_controller.go:108-212`) runs, inside one
transaction (`:152-191`): apply resolution → `tx.Save(&keeper)` → `RepointContactAssociations` →
`deleteContactAssociations(tx, loser, …)` → `tx.Delete(&loser)`.

`RepointContactAssociations` (`backend/services/contact_merge_service.go:362-585`) covers thirteen
association types — notes, reminders, reminder completions, activity links, household members, circle
members, contact tags, field values, life events, conversation agendas, gifts, life-event related-entity
JSON, and relationship edges (with semantic dedupe via `edgesConflict` `:590` / `pickEdgeToDrop` `:606`).

`deleteContactAssociations` (`backend/controllers/contact_controller.go:611-723`) is the canonical
full-cascade checklist. **Anything in it that `RepointContactAssociations` does not handle first is deleted
at `contact_merge_controller.go:175`.** Four categories fall through:

| Lost | Where it's deleted | Note |
|---|---|---|
| **`attachments`** | `contact_controller.go:718` | Worse than a row delete: `contact_merge_controller.go:146-151` plucks the loser's `stored_name`s *before* the transaction and **deletes the files from disk** at `:203`. [N7](29-N7-attachments.md) attachments on a merged-away contact are gone from the filesystem, not just the database. |
| **`preferences`** | `contact_controller.go:675` | [T20a](10-T20a-preferences.md) data. |
| **`cadence_policies`** | `contact_controller.go:696` | [T19](20-T19-cadence.md). Losing it silently changes the keeper's relationship-health signal. |
| **`external_identities` / `external_activities`** | `contact_controller.go:708-712` | [T14](32-T14-external-link-substrate.md), hard-deleted. |
| **Photo** | `contact_merge_controller.go:200` (`deleteContactPhotos`) | If the keeper has no photo and the loser does, the photo is destroyed rather than adopted. |

None of these appear in `ContactMergeAssociationCounts`
(`backend/services/contact_merge_service.go:314-350`), so the merge **preview does not warn about any of
it** either. The user is shown counts for the thirteen things that survive and nothing for the five that
don't.

`contact_sync_links` is also deleted, but that one is deliberate and documented (`contact_controller.go:702`,
note text at `contact_merge_service.go:702-704`) — leave it alone.

## What to build

1. **Repoint what can be repointed.** Add to `RepointContactAssociations`:
   - `attachments.contact_vcard_uid` — plain UPDATE; there is no uniqueness constraint to dedupe against.
     **Do this before the stored-name pluck at `contact_merge_controller.go:146-151`**, or move the pluck
     to after the repoint so it only ever collects files that are genuinely orphaned.
   - `external_identities` / `external_activities` (`entity_id`) — dedupe-then-repoint. Check whether
     `external_identities` has a natural-key unique index on `(provider, external_id, entity_id)`; if so it
     needs the same DELETE-then-UPDATE shape as `circle_members` at `:420-432`.
   - `preferences` (`entity_id`) — plain UPDATE. Duplicate preferences across the two contacts are a
     content-level duplicate, not a constraint violation, and merging them is not this ticket's job.
2. **Decide `cadence_policies` deliberately, don't just repoint.** A cadence policy is one-per-contact and
   the keeper probably already has one. Two policies cannot both survive. Recommended: treat it as a
   **scalar conflict** — add it to the resolution flow the way `ComputeFieldValueConflicts` (`:258-307`)
   handles `FieldValue`'s one-per-contact constraint, so the user picks. If the keeper has none and the
   loser does, adopt the loser's with no prompt.
3. **Adopt the loser's photo when the keeper has none.** `deleteContactPhotos` at `:200` should skip the
   loser's photo in that case, and the keeper's `Photo` path should be updated. Guard it through
   `ApplyRecordToContact` / the `contact_card_merge.go` merge — not a direct field mutation
   (`/CLAUDE.md` backend traps #2 and #3).
4. **Add all of it to `ContactMergeAssociationCounts`** (`:314-350`) so the preview says what will move and
   what will be lost. Anything that still gets destroyed after steps 1–3 must be named in the preview and
   in `BuildContactMergeNoteContent` (`:694-700`).

## Traps

- **Real production data exists.** Already-merged contacts have already lost this; nothing here recovers
  it, and the audit trail can't help — attachments were never in a snapshot.
  [T82](126-T82-audit-snapshots-miss-nested-contact-data.md) captures nested contact data, not associations.
  Say so plainly in the landing note.
- **The on-disk file deletion at `:203` runs outside the transaction.** If the transaction rolls back after
  the files are gone, the attachments are lost with the merge undone. That ordering bug is in scope here.
- `deleteContactAssociations` is the canonical checklist per `/CLAUDE.md` backend trap #6 — **use it as the
  diff source**. Walk every table it names against `RepointContactAssociations` and account for each one,
  rather than fixing only the five found here.
- `/CLAUDE.md` backend trap #4: check `.Error` on every `db.Updates`/`db.Save`.
- `/CLAUDE.md` backend trap #1: test against `database.InitDB`, never `AutoMigrate`.
- Tests must assert with `Unscoped()` where soft-delete is involved — `assertGone`-style
  `db.Model().Count()` helpers exclude soft-deleted rows and pass either way (trap #6).

## Done when

- Merging a contact that has attachments moves them to the keeper, and the files stay on disk.
- Preferences and external identities/activities survive a merge.
- A cadence-policy collision is surfaced as a resolvable conflict; a one-sided policy is adopted silently.
- The keeper adopts the loser's photo when it has none of its own.
- The merge preview lists counts for every association type that moves **and** every one that is destroyed.
- A test walks `deleteContactAssociations`' table list and fails if a table is neither repointed nor
  explicitly listed as intentionally dropped — so the next entity added can't silently regress this.
- `cd backend && go build ./... && go vet ./... && gofmt -l . && go test ./...` green, with every new test
  hand-verified against the reintroduced bug.

## Landing note (2026-08-13)

All four association types plus the photo now survive a merge. `attachments`/`preferences` got a plain
repoint (confirmed neither has a dedupe-worthy unique constraint); the pre-transaction "pluck attachment
stored names, delete files after commit" dance was removed outright rather than reordered, since attachments
are now never deleted by a merge — nothing to race the transaction anymore.

One design correction made while implementing, not just planning: the ticket's "check whether
`external_identities` has a natural-key unique index on `(provider, external_id, entity_id)`" turned out to
resolve the other way. The real index — migration `000005`, `(system, external_id, user_id)` — does **not**
include `entity_id`, which means it unique-constrains *per user*, not *per contact*. Two contacts belonging
to the same user can therefore never separately hold the same `(system, external_id)` pair to begin with —
proven directly: a first attempt at a dedupe-then-repoint test failed at fixture `Create` with a UNIQUE
constraint violation, before a merge was ever involved. So `external_identities`/`external_activities` got a
plain repoint too, with the dead DELETE/EXISTS dedupe branch removed rather than left in as defensive dead
code.

`cadence_policies` (genuinely one-per-contact, migration `000002`'s partial unique index) is the one real new
conflict type. It reuses `ContactMergeResolution.Conflicts` — the same slice scalar field conflicts already
populate — rather than getting its own response field, so `MergeContactsDialog.tsx`'s existing generic radio
UI renders it with **zero frontend changes**. Identical-content policies auto-resolve with no prompt, one
side present adopts silently, and both differing forces a choice exactly like a scalar conflict does.

Photo adoption follows the codebase's own established pattern: mutate the flat `Contact.Photo`/
`PhotoThumbnail` fields directly (same as every other field `ApplyContactMergeResolution` already sets) and
let `BeforeSave`'s T75 merge rederive `Card.Media` on save, rather than touching `Card` directly. The
now-shared file is protected from the post-commit `deleteContactPhotos(c, loser)` call by blanking the local
(in-memory, not DB) `loser.Photo`/`PhotoThumbnail` right after adopting.

Six new/extended real-DB tests in `contact_merge_real_db_test.go` (`database.InitDB`, per `/CLAUDE.md` trap
#1), covering every table `deleteContactAssociations` touches including `NotificationDelivery`. Every new
assertion was hand-verified per `/CLAUDE.md`: temporarily disabled each production fix, confirmed the
corresponding test failed with a message matching the reintroduced bug, restored. `openapi.yaml`'s
`ContactMergeAssociationCounts` schema updated with the five new count fields.

`go build ./... && go vet ./... && gofmt -l . && go test ./...` green.

**Self-review pass (high-effort, 8 parallel finder angles) found four real correctness bugs, fixed in the same
session** — worth recording since two of them are exactly the class of silent-loss bug this ticket exists to
close, just relocated into the new code:

- `repointCadencePolicy` fell through to silently keeping the keeper's policy (deleting the loser's) whenever
  the resolution value didn't exactly match either side's freshly-recomputed summary, instead of erroring —
  e.g. a stale preview value after either policy was edited before commit. Now a hard rejection, matching how
  the `FieldValue` conflict path already treats an unresolved value. New `TestContactMerge_CadencePolicyConflict`
  pair C pins it; hand-verified by reverting the fix and confirming the test fails.
- `formatCadencePolicySummary` joined `QualifyingTypes` without sorting, so the same set of qualifying types in
  a different slice order produced a spurious conflict. Now sorts a copy first. Pair D pins it, same
  hand-verify treatment.
- `appendCadencePolicyConflict` hardcoded the preview-only error message even when called from the commit path.
  Now takes the caller's own message.
- The hand-maintained frontend `ContactMergeAssociationCounts` TS type (`frontend/src/api/contactMerge.ts`,
  whose own comment already says it "must be kept in sync manually") wasn't updated with the five new fields,
  so `MergeContactsDialog`'s `hasAssociations` gate couldn't see them — a merge whose only real effect was one
  of the new association types would show no "this affects associations" notice. Both files updated;
  `tsc --noEmit` and the full `vitest` suite (667 tests) still green.

Also updated `ContactMergeFieldConflict`'s doc comment (now stale — it described only two conflict kinds, not
the cadence-policy one this ticket adds as a third folded into the same `Conflicts` slice) and added a comment
on `ApplyContactMergeResolution`'s silent no-op for that field, so a future same-named scalar field can't
collide with it unnoticed.

Two lower-severity findings surfaced but deliberately left unfixed, with rationale: `BuildContactMergeNoteContent`'s
audit note overstates cadence-policy re-pointing when the loser's was actually discarded on a conflict —
true, but consistent with the same pre-existing imprecision for household/circle/tag dedup counts, so fixing
cadence alone would be inconsistent scope creep; and the soft-deleted loser's DB row keeps a stale `Photo`
value after adoption (in-memory only is blanked) — currently unexploitable, no code path reads it
destructively today.

Not done here, filed separately as out-of-scope-but-noticed: `openapi.yaml`'s `ContactMergeAssociationCounts`
schema was already missing `conversation_agenda_items`/`gift_items` before this ticket (pre-existing drift,
not caught by the drift test since it only checks schema *existence*, not field parity) — flagged as a
follow-up task, not fixed here to keep this diff scoped to T107.
