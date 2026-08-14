# T96 — Import has no merge review: web commits blind, Android doesn't merge at all

| | |
|---|---|
| **Platform** | Web + Android |
| **Rating** | 4 — this is where the duplicates the beta complained about came from |
| **Size** | M (web) + M (Android) |
| **Depends on** | [T93](137-T93-duplicate-scan-endpoint-and-review.md) for the within-batch half; the per-row preview half depends on nothing. |
| **Status** | **DONE** (2026-08-14) |
| **Source** | Beta testing note, 2026-08-13: *"Doing bulk import doesn't give a screen to do a bulk merge of contacts — we should be able to merge duplicates as part of import for VCF, CSV, and Android contacts."* |

## Why this exists

The three import paths behave differently, and none of them does what the report asks.

**Web CSV/VCF/JSContact already has a review step, but it is blind.**
`frontend/src/components/ImportContactsDialog.tsx` steps through `upload → mapColumns → review → done`
(CSV) or `upload → review → done` (VCF), at `:65-66`. The review table (`:536-585`) shows a per-row
duplicate chip with the matched name and reason (`:551-559`) and a per-row Skip / Add as New / Update
Existing select (`:567-581`), seeded from `suggested_action` (`:157-162`, `:237-242`) and with
[T56](65-T56-bulk-contacts-import-flow.md)'s apply-all/skip-all controls at `:260-280`.

What "Update Existing" then does is the problem. `backend/services/import_session.go:267-300` fetches the
matched contact, writes a merge note, and applies `MergeImportedContact` (`:295`) with a fixed
*incoming-wins-if-non-empty* policy. There is **no field-by-field conflict list**, and multi-valued arrays
are **overwritten, not unioned** — the opposite of what the real merge path does
(`unionEmails`/`unionPhones`/`unionAddresses`, `backend/services/contact_merge_service.go:114-232`). The
user is asked to choose "update" without being shown what update will destroy.

**Android doesn't merge at all.** `ImportContactsViewModel.importSelected`
(`android/feature/import/.../ImportContactsViewModel.kt:81-104`) calls
`contactRepository.createContact(input)` **unconditionally** at `:89`. A candidate flagged as a duplicate
at `:22-23` and labelled as one in the UI (`ImportContactsScreen.kt:160-162`) is still created as a brand
new contact. The duplicate flag is purely cosmetic — this is the mechanism that produced the beta's
duplicate pile.

**Neither path detects duplicates *within* the incoming batch.** `DetectDuplicate` compares each row
against the database only, so a VCF containing the same person twice creates two contacts.

## What to build

### Web

1. **A merge preview per duplicate row.** Expand the duplicate chip at `ImportContactsDialog.tsx:551-559`
   into an expandable panel showing which fields differ and what "Update Existing" would do to each. Back
   it with a computation that **reuses `services.ComputeContactMergeResolution`**
   (`backend/services/contact_merge_service.go:50-76`) rather than describing `MergeImportedContact`'s
   separate policy — the point is to converge the two, not document the divergence.
2. **Union multi-valued fields on "Update Existing".** Route the update branch at
   `import_session.go:267-300` through the same `unionEmails`/`unionPhones`/`unionAddresses`/`unionURLs`
   helpers the real merge uses, so importing a contact with a second phone number adds it rather than
   replacing the first. This is the substantive fix; the preview in step 1 is what makes it trustworthy.
3. **Detect within-batch duplicates.** Before rendering the review table, run the incoming rows against
   each other with the same key functions [T93](137-T93-duplicate-scan-endpoint-and-review.md) uses, and
   collapse or flag the matches. A row that duplicates another *incoming* row needs its own action
   ("merge into row N") distinct from "update existing".

### Android

4. **Branch on the duplicate flag.** `ImportContactsViewModel.importSelected:87-98` must offer
   create-vs-update rather than always creating, and `ImportContactsScreen.kt:144-165` must render a
   per-row action (Skip / Add as New / Update Existing) instead of a bare checkbox — mirroring the web
   review table's three choices.
5. **Use the server's detector, not the local one.** `findDuplicate` (`ImportContactsViewModel.kt:63-71`)
   checks only the local Room cache, has no name tier, and calls `findByPhone(number)` with no `PhoneKey`
   normalization — so it misses duplicates the backend would catch and the cache may not even hold the
   contact. Once [T93](137-T93-duplicate-scan-endpoint-and-review.md) exists, call it.

## Traps

- **[T49](58-T49-vcf-import-merge-corrupts-existing-contact.md) is the cautionary tale here** — the import
  merge path has already corrupted and orphaned real contact data once. Re-read its landing note before
  touching `MergeImportedContact`.
- **[T75](119-T75-plain-save-destroys-card-only-data.md)'s rule binds this path.** `MergeImportedContact`
  must not reintroduce a plain `db.Save` on a loaded contact outside the
  `models/contact_card_merge.go` merge — that is exactly how Card-only data got destroyed before.
- `/CLAUDE.md` backend trap #2: never set `Card`/`CRM` by direct field mutation before `Create`; use
  `ApplyRecordToContact`. This bit WP-81 and WP-83 identically.
- Real production data exists. An import that silently overwrites is a data-loss event, not a UX wart.

## Done when

- Importing a VCF whose contact matches an existing one shows, before commit, exactly which fields would
  change and which arrays would gain entries.
- Choosing "Update Existing" for a contact with one phone, importing a card with a different phone, leaves
  the contact with **both** phones.
- A VCF containing the same person twice produces one contact, not two.
- On Android, a device contact flagged as a duplicate can be merged into the existing contact instead of
  creating a second one, and doing so is the default suggested action.
- `cd backend && go build ./... && go vet ./... && gofmt -l . && go test ./...` green.
- `cd frontend && npx tsc --noEmit && npx vitest run` green, plus a Playwright spec covering the
  union-on-update case.
- `cd android && ./gradlew testDebugUnitTest lintDebug assembleDebug` green.
- New strings translated in all five locales, both clients.

## Landing note (2026-08-14)

Built in one session as the described feature — per-contact Merge / Keep Both /
Discard New decision cards during bulk import, on web **and** both Android import
flows. T93 (which this ticket depends on) landed first, so the whole thing was
buildable; T92-as-written (a Merge button in the Contacts list bulk-actions bar)
was deliberately **not** done — it is a separate ticket, and its "Review
duplicates" review surface was already shipped as part of T93.

### Backend

- **`ImportRowPreview` gains `merge_diff` and `batch_duplicate_of`.**
  `merge_diff` describes exactly what the "Merge" (update) action will change on
  the matched existing contact: scalars overwritten (incoming-wins-when-non-
  empty, `ComputeImportMergeDiff`) and multi-valued entries appended (the same
  `mergeContactValues` helpers the confirm path applies — a convergence test
  applies `MergeImportedContact` and asserts the diff predicted every change, so
  the preview can never describe a merge the commit won't perform). Email/Phone/
  Address are reported as additions, not scalar updates — they're `BeforeSave`
  projections of the arrays. Circles/Tags are deliberately absent (membership
  materialization is additive/idempotent, and the flat staging column isn't a
  faithful record of an existing contact's memberships).
- **Within-batch duplicate detection.** `BuildImportRowPreview` (new shared
  preview builder used by the VCF, JSContact, CSV and records-import paths so
  the duplicate/diff/default-action wiring can't drift between formats) compares
  each row against the sibling rows built so far using the same key functions
  as `DetectDuplicate` (email, exact name, phone via `PhoneKey`). A twin gets
  `batch_duplicate_of` pointing at the earlier row and defaults to "skip" — the
  user can still Keep Both.
- **`POST /contacts/import/records`** — a batch of neutral Card/CRM records run
  through the same preview pipeline and confirmed via the shared
  `/contacts/import/vcf/confirm` route. This is the Android device-contacts path
  (T96 step 5's "use the server's detector"): the device flow no longer creates
  each selected contact unconditionally. Session type is `records` so merge
  notes are labelled "Device Import".
- The diff's `updated`/`added` always serialize as `[]`, never `null` (Go emits
  `null` for a nil slice even without `omitempty`; the client renders
  `.length` directly — CLAUDE.md trap #8, pinned by a raw-JSON test).

### Web

The review step of `ImportContactsDialog` is redesigned from a table with a
per-row select into the decision cards: name + match line ("Matches: X (same
email)"), the merge diff ("Will merge: + new phone: ..., Job Title: A → B"), a
within-batch note ("Duplicates row N of this import"), and Merge / Keep Both /
Discard New buttons (Merge only enabled when the row matched an existing
record). "Resolve all as merged" (was "Accept all suggested") + "Skip all"
bulk controls; the apply button is now "Apply Decisions (N)"; the review heading
counts unresolved conflicts ("Resolve Conflicts (N remaining)"). All five
locales updated. `api/import.ts` also fixes `DuplicateMatch.match_reason` to
include `phone`.

### Android

Both import flows share a new `ImportReviewStep` composable with the same
decision cards and diff. The VCF-file flow replaces its Skip/Add/Update chips.
The device-contacts flow becomes select → review → confirm via the new records
endpoint. **Deliberate tradeoff:** the device LOOKUP_KEY linkage (a future
re-import dedup aid, §7.5.4) is dropped — the server owns creation now, and the
client no longer learns which row became which contact. `findDuplicate` still
decorates the list step, but the real duplicate decision is server-side.

### Tests / verification

Backend: pure diff + within-batch unit tests, real-schema `ParseVCF` wiring
tests, records-endpoint controller tests (merge/keep-both/add, within-batch
collapse, empty-batch 400, no-auth 401, raw-JSON `[]` pin) — all hand-verified
to fail pre-fix. Frontend: `ImportContactsDialog.test.tsx` rewritten for the
cards (default-merge, conflict-count countdown, within-batch cannot-merge, bulk
controls), locales test green, and a new `importMergeReview.spec.ts` e2e driving
Merge (diff shown, phone unioned, one contact), Keep Both (two contacts), Discard
New (untouched), and the within-batch collapse against the real UI. Android:
`ImportContactsViewModelTest` (5 tests), a `resolveAll` test, review-step UI
tests. Full gates green: backend `go build/vet/gofmt/test`, frontend
`tsc`+`vitest` (699 tests) + the full 179-test Playwright suite, Android
`testDebugUnitTest lintDebug assembleDebug --rerun-tasks`.

### Post-rebase review pass (2026-08-14)

Rebased onto `main` (which had since landed T88 and T101; the README conflict
resolved by folding both into Done alongside T96). A second review pass fixed
the gaps it found:

- **Web copy bug**: the "No duplicate matches — everything below will be added
  as new." line also appeared when conflicts existed but were all *resolved*
  (which is false once a row merges or discards). Now three states: "Resolve
  Conflicts (N remaining)", "All conflicts resolved — review the decisions
  below.", or the no-matches line only when there never were any.
  `allResolved` translated ×5.
- **a11y**: the Merge / Keep Both / Discard New buttons now carry
  `aria-pressed` so screen readers hear the selected decision.
- **Android stale-list**: the device-contacts RESULT step's Done button now
  calls `startOver()` (reset + reload) so the just-imported contacts' duplicate
  flags reflect fresh server state instead of the stale pre-import cache.
- **Test gaps closed**: CSV within-batch detection (`GenerateCSVPreview`)
  wasn't unit-covered — added; the combined within-batch **and** DB-duplicate
  case (twin defaults to skip while still showing both flags + diff) wasn't —
  added; a Playwright spec for the **CSV** import-merge path (mapping step →
  cards → merge → phone unioned) — added, since only VCF was e2e-covered; and
  the all-resolved heading is now asserted e2e in the Discard test. Suite is
  180 tests green.
