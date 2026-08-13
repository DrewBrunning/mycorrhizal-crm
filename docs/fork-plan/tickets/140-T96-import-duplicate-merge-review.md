# T96 — Import has no merge review: web commits blind, Android doesn't merge at all

| | |
|---|---|
| **Platform** | Web + Android |
| **Rating** | 4 — this is where the duplicates the beta complained about came from |
| **Size** | M (web) + M (Android) |
| **Depends on** | [T93](137-T93-duplicate-scan-endpoint-and-review.md) for the within-batch half; the per-row preview half depends on nothing. |
| **Status** | **TO BE DONE** |
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
