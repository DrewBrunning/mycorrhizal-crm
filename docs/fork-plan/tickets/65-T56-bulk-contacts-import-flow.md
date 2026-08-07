# T56 — Bulk contacts import (Google Takeout / contacts-app export) in Data Settings

| | |
|---|---|
| **Rating** | 3 — real onboarding gap; getting an existing contact set into the app is currently a one-at-a-time or small-batch affair |
| **Size** | M |
| **Depends on** | [T49](58-T49-vcf-import-merge-corrupts-existing-contact.md), [T50](59-T50-vcard21-import-blank-fields.md) — land both first; a bulk flow amplifies both bugs' blast radius (many contacts merged/corrupted at once instead of one) rather than fixing either |
| **Alpha** | n/a — real data exists; reuses existing import machinery, no schema change |
| **Source** | v0.3.0 post-release testing, 2026-08-06 |

## Why this exists

The app already has real VCF/CSV import machinery (`ParseVCF`/`ParseCSV`, the session-based
preview/confirm flow in `import_session.go`), reachable today via an "Import" entry point on the
Contacts list page (`ContactsPage.tsx`) — but it's capped at `MaxVCFContacts`
(`import_service.go:174`), sized for adding a handful of contacts at a time, not importing an
entire existing address book in one pass (a Google Takeout export or another contacts app's full
export can run into the hundreds). There's currently no path to bring in a large existing set
without repeated small batches.

## What to build

- A bulk-oriented import entry point in **Settings → Data** (`DataSettingsPage.tsx`), alongside the
  existing export UI there — per the report, this is where it belongs, not the Contacts list page
  where the existing small-batch import lives today. Decide during implementation whether this
  *replaces* the Contacts-page import (consolidating to one place) or *adds* a second, larger-scale
  entry point — don't leave two import UIs with unclear, overlapping purposes.
- Raise or remove `MaxVCFContacts` for this path, and make sure the existing preview/confirm/
  duplicate-detection UI (`ImportRowPreview`, the whole session flow) scales usably to hundreds of
  rows — a preview list built for a dozen contacts may not be a usable UI at Takeout scale (paging,
  bulk accept-all/skip-all, progress indication for a long-running confirm).
- Reuse the existing `ParseVCF`/`ParseCSV` + session/preview/confirm machinery rather than building
  a parallel bulk-specific pipeline — the point of this ticket is scaling what exists, not
  duplicating it.

## Traps

- This is explicitly gated on T49/T50 landing first — a bulk flow makes both bugs *worse* per run
  (a full address book merged through the broken merge logic, or a full 2.1 export producing
  hundreds of blank-field contacts), not better. Don't start this before those ship.
- Watch for request/response size and timeout limits on a hundreds-of-contacts single upload+parse —
  this may need chunking or a background-job pattern rather than a single synchronous request,
  depending on how large a real Takeout export gets in practice.
- See [T57](66-T57-bulk-import-api-for-external-clients.md) (deferred) for the separate,
  not-yet-scheduled question of a documented API for a future external client (e.g. a mobile app) to
  drive bulk import — this ticket is the in-app UI only.

## Done when

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` green.
- `npx tsc --noEmit` clean, `npx vitest run` green.
- Hand-verified: import a realistically large export (100+ contacts) through the new Data Settings
  flow end to end, including the preview step, without the UI becoming unusable.
- e2e coverage for the new entry point.
- All 5 locale files have real translations for any new strings.

## Landing note (2026-08-07)

Landed. Decision (the ticket's open "replace vs add" question): **add** a bulk entry point on
Settings → Data while **keeping** the Contacts-page one — both doors open the *same*
`ImportContactsDialog`, so there is still exactly one import flow, not two competing UIs.
The wizard now scales to full address-book imports: the preview table paginates client-side
(20 rows/page, so hundreds of rows never mount hundreds of Selects), and bulk "Accept all
suggested" / "Skip all" controls apply one decision across the whole file with a single click.
Confirm shows an explicit progress state (spinner + "Importing…", back/cancel disabled).

Backend caps raised for address-book-scale files: `MaxCSVSize` 5MB→20MB, `MaxVCFSize`
10MB→50MB (photos), `MaxCSVRows`/`MaxVCFContacts` 1000→20000. Pinned by a test that parses
1001 contacts (over the old ceiling) cleanly. e2e drives the real Data Settings dialog end to
end against the all-in-one image.
