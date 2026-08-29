# Meerkat import fixture

A representative Meerkat CRM database for the source import (issue #353), in the reviewable-JSON
form the repo's other shared fixtures use (issue #430's canonical fixture, issue #266's contract
fixtures): the manifest is hand-authored and diff-reviewable; the loader
(`backend/internal/meerkatfixture`) builds a **real Meerkat-schema SQLite file** from it that the
production reader (`backend/meerkat`) consumes exactly like a deployment's database.

## What consumes it

- `backend/internal/meerkatfixture/populate_test.go` — the loader round-trips through
  `meerkat.Open`.
- `backend/services/meerkat_import_test.go` — the full pipeline: manifest → Meerkat DB →
  `meerkat.Open` → `MapMeerkatSnapshot` → `ExecuteSourceImport` into a migrated schema, asserting
  every mapped field lands, direction is preserved, losses are named, and a re-run does not
  duplicate.

## Content

The fixture exercises every mapped concept plus the pathological shapes from TEST-02 (issue #430)
that a Meerkat deployment can express:

- multi-valued emails/phones/urls/impps/addresses, structured name parts, org + department +
  title + role;
- reciprocal relationships (a spouse pair stored from both sides — must collapse to one edge),
  asymmetric typed edges (`Daughter`, `Mentor`), an unrecognized type (`Bff` → `related_to`),
  and a **dangling** relationship (a `name` with no target contact — must be reported);
- circles, custom fields, a food preference, notes, activities (with attendees), reminders;
- Unicode (`Żółć`, `Σωκράτης`), a year-unknown birthday (`--04-20`), a preserved `vcard_uid`, and
  a **very long** free-text value (survives verbatim);
- a **soft-deleted** contact and note (reported `skipped`, not imported);
- a **second source user** (the default import must not mix them in).

## Regenerating / extending

The manifest is hand-authored; edit `testdata/meerkat-fixture/manifest.json` and add/update the
corresponding assertions in the loader and service tests. The fixture is **extended, not
forked** per suite. There is no checked-in `.db` — a binary fixture would be opaque in diffs and
silently stale; the loader builds one fresh per test (`t.TempDir()`).
