# Soak budgets and baseline (issue #1496)

How the soak test works and how to run it is in [soak-testing.md](soak-testing.md).
This page is the committed **threshold table** and a recorded healthy run.

## Budgets

Hand-authored in `backend/internal/soak/budgets.go` (a number and its reason
edit in the same hunk); the table below is **generated** from it by
`cd backend && go run ./cmd/soak -write-budgets-doc ../docs/development/soak-baseline.md`,
and `TestSoakBaselineDoc_BudgetTableIsCurrent` fails until it is regenerated.
Changing a number is a deliberate, reviewed edit that must rewrite its reason.

"Fitted growth" is the least-squares slope of the **last 2/3** of the run
(the first third is warm-up) times the length of that window, so the same figure
governs the 60 s per-PR smoke and the 45 min weekly run; a flat steady state
passes both and a fixed-rate leak shows proportionally more in the longer run.

<!-- soak:budgets:begin -->
| Signal | Judged as | Limit | Gating | Why |
|---|---|---|---|---|
| `goroutines` | fitted growth over the tail window | 12 goroutines | fails the run | A healthy server holds a flat goroutine population (HTTP keep-alive connections, the scheduler, the cleanup ticker, the bounded async-audit and webhook workers). Request- or job-scoped goroutines that never exit (an unbounded fire-and-forget, a leaked client transport) grow by ~1 per leaking operation. 12 absorbs connection-pool jitter and in-flight work at the sample instant. |
| `open_fds` | fitted growth over the tail window | 6 fds | fails the run | Open descriptors track connections plus a few files (db, wal, shm, logs) and are flat in steady state. A leaked response body, file or SQLite connection grows by one per leak. 6 absorbs keep-alive connection churn between two samples. |
| `heap_inuse_bytes` | fitted growth over the tail window | 24 MiB | fails the run | Live heap after a forced GC (in-process mode) is bounded by the working set: the dataset is capped per user, so a growing heap means retained garbage (an unbounded map or slice keyed by request/user, a cache without eviction). 24 MiB clears allocator/size-class noise and the SQLite page cache settling. |
| `rss_bytes` | fitted growth over the tail window | 64 MiB | fails the run | Resident memory follows the heap but also moves with page cache, goroutine stacks and the Go scavenger's release timing, so it is noisier than heap_inuse and given a looser bound (a healthy 60 s run fits under 1 MiB). It is the backstop for memory the Go heap does not account for (cgo, mmap); heap_inuse is the sensitive probe for Go-heap leaks. |
| `wal_bytes` | ceiling (tail max) | 16 MiB | fails the run | SQLite's auto-checkpoint (1000 pages, ~4 MiB) resets the write-ahead log whenever no reader pins it. A healthy run peaks near 4-5 MiB; a WAL past 16 MiB under this workload is not being checkpointed — a long-lived read transaction (checkpoint starvation) or a disabled auto-checkpoint — and would grow without bound on a real instance. |
| `ratelimiter_entries` | fitted growth over the tail window | 20 keys | fails the run | Per-IP and per-account limiter maps are evicted by the 5-minute cleanup ticker. The workload keeps its client set and its failed-login identifiers fixed, so the tracked-key count must be flat; per-request growth means state keyed by something unbounded that never evicts. 20 allows the failed-login lockout entries and known-good-IP records to settle. |
| `db_connections_open` | fitted growth over the tail window | 6 connections | fails the run | The database/sql pool is not capped (no SetMaxOpenConns), so the open-connection count tracks concurrent in-flight requests: a starved CI runner piles requests up and legitimately peaks well above the steady state (27 observed with 24 in flight, back to 2 at the end). A ceiling therefore measures contention, not leaks. A leaked *sql.Rows/*sql.Tx pins its connection forever, so the count climbs over the run instead of returning to the pool; judged as fitted tail growth, like open_fds. 6 absorbs in-flight work at the sample instant. |
| `db_bytes` | fitted growth over the tail window | 96 MiB | advisory | The database legitimately grows during a run — every request writes audit rows, soft-deleted contacts stay recoverable until the retention purge, and the FTS index tracks live rows — so this is reported, not failed. It is bounded loosely so a runaway (a write loop, an un-purged table) is still visible in the report. |
| `p95_read_s` | degradation (tail vs first third) | 4 x + 0.25 s | fails the run | Read latency must not degrade with process lifetime: the tail median p95 may be at most 4x the first third's plus 250 ms. This is a relative growth class, not a wall-clock budget — CI hardware speed cancels out — and catches an index/derived-data decay (FTS fragmentation, a growing scan) that only shows after many writes. |
| `p95_write_s` | degradation (tail vs first third) | 4 x + 1 s | fails the run | Writes serialise on the single SQLite writer, so their p95 is dominated by queueing and fsync and is noisier; the allowance is 4x the first third's plus 1 s. A WAL that stops checkpointing or a lock held across a request shows here as a ratchet. |
| `p95_auth_s` | degradation (tail vs first third) | 4 x + 1 s | fails the run | Login/logout are bcrypt-bound (deliberately slow, CPU-contended with the rest of the workload), so the allowance is 4x the first third's plus 1 s. Growth beyond that means session/limiter state is accumulating on the auth path. |
<!-- soak:budgets:end -->

## Recorded healthy run (indicative, not drift-gated)

Measured on the development machine, in-process real server, 180 s at 20 ops/s
across 10 users, sampled every 3 s (`go run ./cmd/soak -duration 180s -sample 3s`).
Wall-clock and byte figures vary with the machine, which is why they are context
and not a gate (the same stance as `perf-benchmarks.md`).

| Signal | Start | End | Tail growth | Budget |
|---|---|---|---|---|
| goroutines | 12 | 14 | 0.04 | 12 |
| open fds | 45 | 47 | 0 | 6 |
| heap in use (after GC) | 6.2 MB | 7.2 MB | 0.1 MB | 24 MiB |
| RSS | 55.0 MB | 57.7 MB | -0.3 MB | 64 MiB |
| WAL | 2.5 MB | 4.3 MB | 0 (ceiling 4.3 MB) | ceiling 16 MiB |
| rate-limiter keys | 22 | 33 | 0 (saturated) | 20 |
| p95 read / write / auth | 5 ms / 5 ms / 50 ms | same | none | 4x + floor |

Workload: 3,746 operations, 0 server errors, 17 `429`s (the production auth
limiter throttling repeated bad logins, as designed). End checks: scheduler 17
jobs with future next runs, 50 live / 50 deleted names correct in search,
`integrity_check`/`foreign_key_check` ok, FTS rows equal live rows in all three
indexes, doctor pass with no violations.

Harness self-verification (leaks injected, run expected to fail), 30 s:
`-fault goroutines,heap,fds,limiter` breached goroutines (+499 vs 12), open fds
(+60 vs 6), heap (+81 MiB vs 24), RSS (+82 MiB vs 64) and rate-limiter keys
(+163 vs 20); `-fault wal` (a pinned read snapshot) drove the WAL to 35 MiB
against the 16 MiB ceiling.
