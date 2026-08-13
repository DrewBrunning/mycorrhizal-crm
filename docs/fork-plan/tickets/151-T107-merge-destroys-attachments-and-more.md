# T107 — Merge permanently destroys the loser's attachments, preferences, cadence policy and external links ⚠

| | |
|---|---|
| **Platform** | Backend |
| **Rating** | 5 — silent, unrecoverable data loss on real production data |
| **Size** | M |
| **Depends on** | Nothing. **Should land before** [T92](136-T92-bulk-merge-from-contacts-list.md), which makes merge much easier to reach. |
| **Status** | **TO BE DONE** |
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
