# T95 — After a merge the keeper shows its old circles — the backend carried them, the UI didn't refetch

| | |
|---|---|
| **Platform** | Web (+ a backend regression test) |
| **Rating** | 4 — reads as data loss on a destructive operation, which is the worst way to be wrong |
| **Size** | XS |
| **Depends on** | Nothing. Same root cause as [T94](138-T94-merge-dialog-stays-open.md); fix both together. |
| **Status** | **DONE**, 2026-08-13. Frontend-only fix plus a backend pin. `onMerged` now awaits `refreshCircles()` and `refreshTags()` before navigating, using the same handles the add/remove paths already call. **The backend was never wrong** -- and the existing `TestContactMerge_RealMigratedSchema` already covered the plain repoint, contrary to this ticket's first draft. What it did not cover is the dedup half: it seeds the circle and tag on the loser only, so the `DELETE`-the-duplicate leg at `contact_merge_service.go:420-446` never executed. New `TestContactMerge_SharedCircleAndTag_Deduped` seeds a shared circle+tag *and* a loser-only one of each, so a fix that deleted instead of repointing cannot pass it. Hand-verified both legs: removing the dedup `DELETE` makes it fail with a 500 (unique-constraint violation), removing the repoint `UPDATE` makes both this test and the pre-existing one fail on counts. |
| **Source** | Beta testing note, 2026-08-13: *"Merge doesn't carry forward circles."* Investigation found the backend does carry them. |

## Why this exists

**The backend is correct.** `RepointContactAssociations`
(`backend/services/contact_merge_service.go:362-585`) re-points circle memberships and tags with a
dedupe-then-repoint pair, at `:420-432` and `:434-446` respectively:

```sql
DELETE FROM circle_members WHERE member_vcard_uid = <loser> AND circle_id IN
  (SELECT circle_id FROM circle_members WHERE member_vcard_uid = <keeper>);
UPDATE circle_members SET member_vcard_uid = <keeper> WHERE member_vcard_uid = <loser>;
```

Both are also counted in the merge preview (`ComputeContactMergeAssociationCounts`, `:325-326`) and named
in the audit note (`BuildContactMergeNoteContent`, `:694-700`). The legacy flat `Contact.Circles` JSON
column is separately unioned by `unionCircles` (`:233`).

**The frontend is what's wrong.** `ContactDetailPage` derives the contact's circles from the
`useCircles()` hook's list (`frontend/src/ContactDetailPage.tsx:325-329`), filtered by `record.uid`. Every
other mutation path refreshes it — `handleCircleAdd` and `handleCircleRemove` both `await refreshCircles()`
(`:841-867`), including in their error branches.

The merge path does not. `onMerged` is `navigate(\`/contacts/${keeperId}\`)` (`:1386`), and because
`/contacts/:id` renders the same element (`frontend/src/App.tsx:486`) the component never unmounts, so
`useCircles`' data is never re-fetched. The keeper renders with the membership list as it was *before* the
merge — which looks exactly like the circles having been dropped.

## What to build

1. In `ContactDetailPage.tsx:1386`, `onMerged` must `await refreshCircles()` and the tag equivalent (and
   close the dialog, per [T94](138-T94-merge-dialog-stays-open.md)) alongside the navigate. Use the same
   `refreshCircles` the add/remove handlers at `:841-867` already call.
2. **Add a backend real-DB regression test for the dedupe branch.** `TestContactMerge_RealMigratedSchema`
   (`backend/controllers/contact_merge_real_db_test.go:270-275`) already pins the plain repoint — but it
   seeds the circle and tag on the **loser only**, so only the `UPDATE` half of the DELETE-then-UPDATE pair
   at `contact_merge_service.go:420-446` is ever executed. A circle **both** contacts belong to must end up
   with exactly one membership row for the keeper, not a duplicate and not a constraint error. Use
   `database.InitDB(filepath.Join(t.TempDir(), "x.db"))` per `/CLAUDE.md` backend trap #1.

## Traps

- **Hand-verify the new test.** Delete the `circle_members` UPDATE at
  `contact_merge_service.go:428` and confirm the test fails; restore. A test that has never failed has
  proven nothing (`/CLAUDE.md`).
- Assert with `Unscoped()` where it matters — `circle_members` hard-deletes (`/CLAUDE.md` trap #7), but the
  `assertGone`-style `db.Model().Count()` helper excludes soft-deleted rows and would pass either way for
  anything that doesn't (trap #6).
- The same missing-refresh shape applies to anything else the detail page holds outside `record` — check
  households and relationship edges before assuming circles and tags are the only two.

## Done when

- Two contacts in different circles, merged: the keeper's header shows **both** circles immediately, with
  no manual reload.
- Two contacts in the *same* circle, merged: the keeper has exactly one membership row for it.
- The same for tags.
- The backend test is hand-verified to fail against the removed UPDATE.
- `cd backend && go build ./... && go vet ./... && gofmt -l . && go test ./...` green.
- `cd frontend && npx tsc --noEmit && npx vitest run` green, plus a Playwright spec driving the real merge
  UI and asserting the post-merge chip set.
