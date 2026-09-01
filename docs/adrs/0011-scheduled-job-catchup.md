# ADR 0011: Scheduled-job catch-up semantics — fire missed occurrences once, de-duplicated

- **Status:** accepted
- **Date:** 2026-09-01
- **Depends on:** ADR 0005 (operational-event model), ADR 0010 (idempotency keys — this is the same
  "retry cannot duplicate" reasoning applied to timer-driven work)
- **Implements:** issue #526. The `v0.6.7` milestone criterion covered here: reconnection/restart
  cannot silently drop *or* double a scheduled occurrence.
- **Feeds:** #391 (persist background-job outcomes) and #422 (notification-delivery health) get a real
  source — `JobExecution.LastOutcome` — instead of log-scraping. Related: #482 (DATE-01 temporal
  semantics).

## Context

The scheduler is in-process `gocron` with **no persistent schedule**. `s.Every(1).Day().At(...)`
computes the *next* occurrence from process start; a fire time the process slept through is never
replayed. What actually rescues a missed run is the paired boot-time trigger next to every scheduled
job — `go safeGo("<name>", ..., Initial, task)` — which fires each job immediately on start, and
`acquireJobLock(db, jobName, minInterval)`, which suppresses that boot run if the job's `LastRunAt`
is inside a per-job de-dup window.

So "catch up on next start, de-duplicated" was already the *emergent* behaviour — but it was nowhere
written down, nowhere tested as a policy, and the windows disagreed:

| Job | old `minInterval` |
|---|---|
| `daily_reminders` | **1h** ← a daily job with a 1-hour window: two restarts an hour apart re-entered `SendReminders` |
| `cadence_overdue`, `reach_out_detection`, `audit_purge`, `system_event_purge`, `job_run_purge`, `purge_deleted`, `storage_sample` | 23h (seven copy-pasted constants) |
| `calendar_sync`, `db_integrity_check`, `restore_drill` | period − 30m (three separate margin constants + floor functions) |
| `alert_eval` | period − 1m |
| `immich_sync` | flat 30m regardless of `IMMICH_SYNC_INTERVAL_HOURS` |
| `webhook_retries` | flat 4m |

`daily_reminders`' `notificationDeliveryKey` set — the per-`(reminder, channel)` guard — was the only
thing between the 1h window and a duplicate email, and it had only ever been tested on the single-run
path, never the restart path.

## Decision

### The policy — catch-up with de-duplication

1. **A missed occurrence fires at the next opportunity** — process start (the `Initial` trigger) or
   the next scheduled tick, whichever comes first.
2. **At most one logical occurrence per job per scheduled period runs.** Two missed daily occurrences
   produce one run, not two — the second is suppressed as "ran too recently".
3. **De-duplication is ultimately on the user-visible event, not only the job.** A run that would
   emit a notification / webhook / suggestion identical in kind and subject to one already emitted
   inside the window is suppressed at that layer too. For reminders this is `notificationDeliveryKey`
   (`(reminder, channel)` with `status='sent'`); this ADR pins it against the restart path.
4. **Suppression is recorded, not silent.** `JobExecution.LastOutcome` (migration 000048) carries
   `ran` / `deduped` / `caught_up` / `failed` (transiently `running` while the lock is held), and
   the main.go job wrapper already records a `skipped` `job_runs` row for the same events (#391). An
   operator can tell a caught-up run from a suppressed one from a normal run.

### The de-dup window — one derivation, one margin

`minInterval` is **derived from the job's scheduled period**, not hand-picked per service:

```
JobCatchupWindow(period) = period − margin
    where margin = min(JobCatchupMargin, period/4)     // JobCatchupMargin = 30m
```

Subtracting a margin means the scheduled tick that fires right after a boot `Initial` run is still
inside the window and suppressed, while a genuinely missed occurrence (a full period has elapsed) is
outside it and runs. The `period/4` clamp keeps a sub-hour job's window close to its period instead
of collapsing. **One constant (`JobCatchupMargin`) for the whole fleet**, replacing the six
hand-rolled ones.

Per-job windows after this change:

| Job | scheduled period | de-dup window |
|---|---|---|
| `daily_reminders`, `cadence_overdue`, `reach_out_detection`, `audit_purge`, `system_event_purge`, `job_run_purge`, `purge_deleted`, `storage_sample` | 24h | 23h30m |
| `idempotency_key_purge` | 6h | 5h30m |
| `calendar_sync` | `CALDAV_SYNC_INTERVAL_HOURS` (default 6h) | period − 30m |
| `immich_sync` | `IMMICH_SYNC_INTERVAL_HOURS` (default 6h) | period − 30m |
| `db_integrity_check` | `DB_INTEGRITY_CHECK_INTERVAL_HOURS` (default 24h) | period − 30m |
| `restore_drill` | `DB_RESTORE_DRILL_INTERVAL_HOURS` | period − 30m |
| `alert_eval` | `ALERT_EVAL_INTERVAL_MINUTES` (default 15m) | period − period/4 |

### `webhook_retries` is deliberately exempt

`webhook_retries` runs every 5 minutes and is a **continuous retry poller**, not a periodic
occurrence with a user-visible event. A missed tick is covered by the next one; each pass is
idempotent by construction (it only re-sends deliveries still marked for retry, and
`ProcessWebhookRetries` holds its own job lock). It keeps its flat 4-minute restart-debounce — the
catch-up-with-dedup policy does not apply to it, and forcing a period-derived window on a 5-minute
job would over-suppress legitimate retries.

### How `caught_up` is detected

`acquireJobLock` marks a run `caught_up` when it acquires the lock and the row has sat idle for
`>= 2 * minInterval` — roughly two full periods, an unambiguous "the process was down long enough to
miss an occurrence" signal that never mislabels a normal cadence run (whose `LastRunAt` is ~one
period back). This under-reports the exact "missed exactly one occurrence" case as a plain `ran`;
that is accepted. An exact per-occurrence ledger would need the scheduled-occurrence timestamp
threaded through the lock, which is #482 (DATE-01) territory, not this ticket's.

## Consequences

- `daily_reminders` now has a 23h30m window: two restarts within a day cannot re-enter
  `SendReminders` and produce a second digest. The `notificationDeliveryKey` guard is the belt to
  that new suspenders, and is now tested against the restart path.
- Six duplicated `…MinInterval` constants and three bespoke floor functions collapse into
  `JobCatchupWindow` + `JobCatchupMargin`.
- `immich_sync`'s window now scales with its configured cadence instead of being a flat 30m.
- `JobExecution.LastOutcome` makes rule 4 real without a new table; #391/#422 can read it.
- No API surface changes — `JobExecution` is internal lock state, not serialized anywhere.
- A future exact-occurrence model (rule 2 at single-occurrence granularity, and a true `caught_up`
  signal) is left to DATE-01; the `>= 2*minInterval` heuristic is the pragmatic stand-in.
