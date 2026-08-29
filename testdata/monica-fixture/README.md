# Monica import fixture

A representative Monica account snapshot for the source import (issue #351). It is the shape the
live API fetch produces (`monica.Snapshot`, defined in `backend/monica`), checked in as
reviewable JSON — the same "one canonical copy, each consumer points its own loader at it"
arrangement as the contract fixtures (issue #266).

## What consumes it

- `backend/services/monica_import_test.go` — the full pipeline: `monica.LoadSnapshot` →
  `MapMonicaSnapshot` → `ExecuteSourceImport` into a migrated schema, asserting every mapped field
  lands, direction is preserved, losses are named, and a re-run does not duplicate.
- `backend/monica/types_test.go` — the loader parses the fixture shape.

## Content

The fixture exercises every mapped concept plus the pathological shapes from TEST-02 (issue #430)
that a Monica account can express:

- rich contacts (birthday, career, how-you-met, description, food preferences, addresses, mixed
  contact fields routed to email/phone/url/impp, avatar photo + gravatar);
- a year-unknown birthday (`--04-20`), a **deceased** contact (death anniversary), a **starred**
  contact (`IsFavorite`), and a **partial** contact (reported `skipped`);
- relationships written bidirectionally (a spouse pair — must collapse to one edge) and
  asymmetrically (`Daughter`, `Colleague`);
- activities, notes, reminders (a birthday reminder and a past one-time dead row), a logged call,
  a task, a debt, and gifts (`idea` and `offered` → `given`).

## Regenerating / extending

Hand-author `testdata/monica-fixture/snapshot.json` (it is a plain JSON serialization of
`monica.Snapshot`), then add/update the assertions in the service tests. The fixture is
**extended, not forked** per suite. A snapshot is a static artifact: the live-fetch client that
produces one from a running instance is the deferred #549 ticket, and this file stands in for its
output so the mapping is provable today.
