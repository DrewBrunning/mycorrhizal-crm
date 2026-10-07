# Soak (long-run) testing (issue #1496)

Every other test and benchmark here is short-lived. The product is a
long-running, single-process, SQLite-in-WAL service with scheduled jobs, so the
failures that only appear with process lifetime need their own test: WAL
growth / checkpoint starvation, memory, goroutine and file-descriptor growth,
rate-limiter map growth, FTS index drift after churn, a wedged scheduler, and
latency that degrades with age. The soak runs a sustained mixed workload and
asserts on **growth classes**, never on wall-clock speed.

## What runs

`backend/internal/soak` (library) + `backend/cmd/soak` (CLI).

- **Server**: by default the real server is booted in-process through
  `embedded.Start` — the same entry point `main()` uses — on a loopback port
  against a fresh migrated SQLite file, with at-rest encryption, the scheduler
  (including the boot-time catch-up burst), the rate-limit cleanup ticker and
  `/metrics`. `-target URL -metrics-token T [-db path]` soaks an already-running
  server instead (an end-of-run database check needs the file to be visible).
- **Workload** (default 20 ops/s across 10 users, seeded): contact list / get /
  create / update / delete, audit undo of an update, FTS search, notes,
  activities, CSV export, VCF import (upload + confirm), login/logout cycles and
  failed logins against a fixed identifier pool. The per-user dataset is capped
  so the database and FTS index reach a steady state rather than growing for
  the whole run.
- **Sampling** (every 5 s, 30 s in the weekly run) from `/metrics`: RSS, heap in
  use (after a forced GC in-process), goroutines, open fds, WAL bytes, database
  bytes, tracked rate-limiter keys, open DB connections, 5xx and job-failure
  counters, and p95 latency per route class (read / write / auth) differenced
  from the request-duration histogram.

The signals `process_resident_memory_bytes`, `process_open_fds`,
`mycorrhizal_ratelimiter_entries{limiter}` and `mycorrhizal_storage_bytes{kind="wal"}`
are new gauges added for this (RSS and fds are Linux `/proc` readings and are
absent elsewhere).

## What fails the run

1. **Budgets** ([soak-baseline.md](soak-baseline.md), `internal/soak/budgets.go`):
   fitted growth over the tail window (last 2/3) for goroutines, fds, heap, RSS,
   rate-limiter keys; a ceiling for the WAL and open DB connections; a
   degradation ratio (tail median vs first third) for latency. A required signal
   that is missing from `/metrics` fails — a gauge that silently stopped being
   exported must not read as "no growth".
2. Any **5xx** or transport error, a failed scheduled-job run, or more than 5 %
   of operations rate-limited (then the run measured throttling, not the server).
3. End-of-run checks: `PRAGMA integrity_check` / `foreign_key_check`; **FTS row
   counts equal live row counts** in all three indexes; a **search freshness
   probe** (a created-and-untouched contact is found by name, a deleted one is
   not — counts can match while content has drifted); the **doctor** data-integrity
   pass (`services.RunDataIntegrityChecks`, what `cmd/doctor` runs); and the
   **scheduler** check (every job registered, none errored, every next run in the
   future).

## Forms

| Form | Where | Shape |
|---|---|---|
| Per-PR smoke | `go test ./internal/soak ./cmd/soak` in the required `Backend (Go)` job | ~14 s run, 1 s sampling; healthy run must pass, injected leaks must fail |
| Weekly / manual | `.github/workflows/soak.yml` (Sundays 04:25 UTC + `workflow_dispatch`) | 45 min, 30 s sampling; advisory tier (not a required context, not a release gate); registered with `nightly-failure-alert.yml` |
| Local | `cd backend && go run ./cmd/soak -duration 60s -sample 2s` | prints the markdown report; `-json` writes every sampled series |

The weekly run publishes the report to the step summary and uploads the full
series JSON as the `soak-report` artifact (the data behind the graphs).

## Proving the assertions bite

`-fault goroutines,heap,fds,limiter,wal` injects deliberate leaks into the
in-process server (real rate limiter, real connection pool; the WAL fault pins a
read snapshot so SQLite cannot checkpoint). A faulted run must exit non-zero;
`TestRun_InjectedLeaksAreDetected` and `TestRun_WALStarvationIsDetected` keep
that proof in CI. `workflow_dispatch` takes a `fault` input to do it on a runner.

## Limits (what this does not cover)

- It is not the shipped image: the process boundary (nginx + supervisord in the
  all-in-one container) is not present, and `docker stats` is not sampled. The
  external-target mode can soak a container but cannot GC-normalise its heap.
- **Aged time is not simulated.** DST/day-boundary scheduler behaviour, session
  and device-grant expiry, and API-token `last_used` throttling need an
  injectable clock; that is issue #1494. Until it lands the run is real time, so
  the scheduler assertions are "alive, no failures, next runs in the future".
- Job *outcomes* come from `job_runs_total`; with most jobs hourly/daily, a
  45-minute run sees only the boot-time catch-up executions.

## Findings

The first runs surfaced a real bug, fixed in the same change:
`checkCanonicalRecords` (the doctor / scheduled data-integrity pass) scanned the
`card` column raw, so with at-rest encryption armed — always, in a running
server — every contact read as invalid JSON. It now decrypts first
(`TestDataIntegrity_INV_D8_EncryptedCardIsNotInvalidJSON`).
