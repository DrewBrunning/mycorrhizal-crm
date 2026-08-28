# Observability: logs, correlation IDs, and the system-event timeline

Where to look when something is failing in production, and how the pieces connect. Companion to
`docs/deployment.md` (install/backup/restore) and `docs/security/data-retention-lifecycle.md` (how
long each of these records survives).

| | |
|---|---|
| **Last updated** | 2026-08-27 (issues [#423](https://github.com/DrewBrunning/mycorrhizal-crm/issues/423), [#424](https://github.com/DrewBrunning/mycorrhizal-crm/issues/424), [#425](https://github.com/DrewBrunning/mycorrhizal-crm/issues/425), [#426](https://github.com/DrewBrunning/mycorrhizal-crm/issues/426), [#427](https://github.com/DrewBrunning/mycorrhizal-crm/issues/427)) |
| **Audience** | Operators diagnosing a failure without local reproduction. |
| **ADR** | [`docs/adrs/0005-operational-event-model.md`](../adrs/0005-operational-event-model.md) |

## The one-pass diagnostics sweep

`GET /admin/diagnostics` (admin-only) is the "is this install healthy?" run
(issue [#423](https://github.com/DrewBrunning/mycorrhizal-crm/issues/423)) — the
manual, one-pass sweep for after an install, an upgrade, a migration, or a
config change. It folds the same single-check paths the surfaces below use into
one `ok` / `warning` / `error` checklist with a summary
(`"summary": {"status": "warning", "ok": 15, "warnings": 2, "errors": 1}`):

| check | what it validates |
|---|---|
| `config` | `config.Validate()` — the same gate boot enforces; only the *names* of failing variables are reported, never values |
| `database` / `migrations` | the deep-health DB read probe + migration lag |
| `filesystem` | writability of the profile-photo, attachments, and database directories (where backups land by default), plus the FCM service-account file |
| `backup` | the persisted restore-drill result (the latest backup-validity proof); disabled = ok, stale/failed = warning |
| `notification_<channel>` | per-channel delivery health (#422): `failing` = error, `no_devices` = warning, `unconfigured`/`healthy` = ok |
| `integration_<system>` | CardDAV/CalDAV (served locally — enabled is reachable); the distinct configured Immich/Paperless/Seafile/Nextcloud base URLs probed with a 2 s timeout, honoring each integration's block-private-URLs SSRF policy |
| `disk_space` | the alert evaluator's statfs fold (#428): warning at the configured threshold (default 90%), error when essentially full |
| `background_jobs` | the job-run-health fold (#391) + deep-health lock liveness: any failing job or stuck lock = warning |
| `version` | build version / commit / build date |

It is read-only and secret-free: config values are never echoed, and
integration base URLs / transport errors go to the log only. Every check is
time-bounded and the whole sweep runs under a total budget, so an unreachable
remote cannot hang it. A broken install still returns HTTP 200 — the diagnosis
**is** the payload, so the checklist is the source of truth, not the status
code.

## The three places diagnostics live

1. **The structured log stream** — `zerolog` JSON on stdout, captured by your Docker log driver.
   Every line is one JSON object. This is the detailed, high-volume record; it is *not* stored by the
   app and rotates according to your log driver's config.
2. **The system-event timeline** — a persisted table (`system_events`) of discrete operational state
   transitions, surviving restarts, viewable at **`/system-events`** (web, admin-only) or **Settings →
   System events** (Android, admin-only). This is the low-volume "what happened to the instance"
   record: app start/stop, scheduled job runs, sync runs, backup/restore drills, migrations.
3. **The audit trail** — `/audit`, a *different* record: who changed user-authored CRM data. Not
   operational. See `docs/security/data-retention-lifecycle.md` §3.

## Log field vocabulary

Operational log lines use a standard set of keys (`backend/logger/fields.go`) so the stream can be
filtered the same way everywhere:

| field | meaning |
|---|---|
| `event` | what happened — e.g. `job_completed`, `sync_failed`, `migration_completed` |
| `component` | subsystem — `scheduler`, `contact_sync`, `calendar_sync`, `notification`, `webhook`, `backup`, `migration`, `app` |
| `operation` | the specific unit of work — usually a job name |
| `duration_ms` | wall-clock duration of the operation |
| `result` | `success` \| `failure` \| `skipped` (`skipped` = job lock held / feature disabled / nothing to do) |
| `error` | sanitized, length-capped error string, present on the failure path |
| `correlation_id` | the ID shared by every step in one chain of work (see below) |
| `request_id` | the originating HTTP request's ID (equal to `correlation_id` for request-driven work) |
| `user_id` | the acting user, when the work is attributable to one |

Not every log line carries all of these — request-path logging (`middleware/logging.go`) has its own
shape, and low-value debug lines are left alone. The rule is *consistency at the operational
producers*, not a blanket migration.

## Correlation IDs: following one action to its outcome

A single unit of user-visible work gets **one correlation ID**, and every background step and
outbound call it spawns carries the same ID.

- **HTTP requests**: the request ID (`X-Request-ID`, generated if the client didn't send one) is
  bound as the correlation ID on the request context. Anything a handler starts from
  `c.Request.Context()` — a CardDAV/CalDAV sync, an outbound call — inherits it.
- **Outbound calls**: the contact/calendar sync HTTP clients set `X-Correlation-ID` on the outgoing
  request, so a remote server's access log can be tied back to the action that triggered it.
- **Scheduled jobs**: each run gets a fresh ID of the form `job:<job-name>:<uuid>` (minted by the
  `runJob` wrapper in `backend/main.go`). Every line the job emits, and every outbound call it makes,
  shares it.

**To trace a UI action to its result:**

1. Grab the correlation ID. In the browser: the `X-Request-ID` response header on the action's
   request (DevTools → Network). In the logs: the `correlation_id` on the `HTTP request` line for
   that path/time.
2. Grep the log stream for it: `docker logs <container> 2>&1 | grep '"correlation_id":"<id>"'` (or
   your log driver's equivalent). Every line of the chain — the request, the service work, the
   outbound call, the eventual `*_completed` / `*_failed` — carries that one ID.
3. For persisted operational events, open `/system-events`, paste the ID into the **Correlation ID**
   filter (or click **View related** on any event in the chain). This lists every `system_events` row
   for that chain, chronologically.

There is no in-app log browser: raw-log storage and search are your log driver's job on a
self-hosted, single-process deployment. The timeline's drill-down is a pivot across `system_events`
by correlation ID, plus the log grep above.

## The system-event timeline

`GET /admin/system-events` (admin-only), rendered at `/system-events` (web) and Settings → System
events (Android). Reverse-chronological, filterable by `component`, `severity`
(`info`/`warn`/`error`), `event_type`, `correlation_id`, and a time range.

Event types (the full `CHECK`-constrained set is in `backend/models/system_event.go` and migration
`000038`): `application_started`/`application_stopped`, `migration_started`/`migration_completed`/
`migration_failed`, `job_started`/`job_completed`/`job_failed`, `sync_started`/`sync_completed`/
`sync_failed`, `notification_sent`/`notification_failed`, `backup_completed`/`backup_failed`/
`restore_test_completed`, `integration_failed`.

Which producers emit today:

| producer | events |
|---|---|
| Scheduler (`runJob`) | `job_failed` on a recovered panic; ordinary start/finish are log lines only (the scheduler ticks often — a per-tick row would swamp the timeline) |
| Contact / calendar sync | `sync_completed` (with counts) / `sync_failed` (with the classified error) |
| Restore drill (issue #275) | `restore_test_completed` on a healthy run; `backup_failed` (`severity=error`) on a failed or mismatched run |
| Migration runner | `migration_completed` (with the from/to version) when the schema actually advanced |
| Notification dispatch | `notification_sent` / `notification_failed` per channel (`detail` = channel name only) |
| Webhook delivery | `integration_failed` once a delivery exhausts its retry budget and is still failing; the outbound POST also carries `X-Correlation-ID` |
| Process | `application_started` (with build version) / `application_stopped` |

The per-delivery *detail* tables (`notification_deliveries`, `webhook_deliveries`) still exist — the
`system_events` row is the timeline summary, not a replacement. Per-channel delivery *health*
(rates, last-good state) is [#422](https://github.com/DrewBrunning/mycorrhizal-crm/issues/422). The
ntfy/Gotify/FCM push channels do not yet set an outbound correlation header (no standard one exists);
their events and log lines carry the ID.

**Retention**: `SYSTEM_EVENT_RETENTION_DAYS` (default `30`). A daily purge job
(`system_event_purge`) hard-deletes rows older than the window; `<=0` disables the purge (it never
deletes everything). Rows are present in a backup and are only as fresh as the snapshot.

## Per-subsystem health (last-known-good)

`GET /admin/subsystem-health` (admin-only) rolls the timeline up into a **last-known-good** state per
subsystem (issue [#427](https://github.com/DrewBrunning/mycorrhizal-crm/issues/427)): for
`contact_sync`, `calendar_sync`, `notification`, `backup`, `scheduler`, and `webhook` it reports the
current `status` (`healthy` / `failing` / `unknown`), `last_attempt_at`, `last_success_at`,
`last_failure_at`, `incident_first_failure_at` (the first failure of the current unbroken run),
`consecutive_failures`, and the most recent `last_error`. "A sync failed" becomes "the last success
was 17:04, it has failed 9 times in a row, and this incident started at 17:19".

It is **derived on read** by folding `system_events` (each `*_completed` / `*_failed` / `*_sent` row
for the subsystem's `component` advances the fold) — there is no second write path and no stored
state, so it survives a restart for free and can never drift from the events it summarizes. Each
`*_completed`/`*_failed` event advances the state; a success resets `consecutive_failures` to 0 and
closes the incident.

**Limitation**: `scheduler` and `webhook` emit only a *failure* event today (`job_failed` on a
recovered panic; `integration_failed` once a delivery exhausts its retry budget). They can therefore
report `failing` / `unknown` and a rising `consecutive_failures`, but never `healthy`, and their
incident cannot auto-close until a success-side event exists — webhook via
[#422](https://github.com/DrewBrunning/mycorrhizal-crm/issues/422); the scheduler intentionally does
not emit `job_completed` on the timeline (a per-tick row would swamp it) — the per-run record lives
in `job_runs` instead (next section). The token sets are lists, so a future `job_completed` /
`backup_completed` producer needs no code change.

Consumers of this state: `/metrics`
([#389](https://github.com/DrewBrunning/mycorrhizal-crm/issues/389)) exports the counters as gauges;
alerting on `Healthy`↔`Failing` transitions
([#428](https://github.com/DrewBrunning/mycorrhizal-crm/issues/428), *Alerting on state transitions*
below) reads it directly. Error aggregation (next section) is a *sibling* fold over the same stream,
not a consumer of this one. Rendered at `/system-events` (web) and Settings → System events
(Android), above the event timeline.

## Background job run history

`GET /admin/job-runs` and `GET /admin/job-runs/health` (both admin-only) are the per-run record for
the scheduled jobs (issue [#391](https://github.com/DrewBrunning/mycorrhizal-crm/issues/391)) that
`system_events` deliberately omits. Every invocation of every scheduled job — `daily_reminders`,
`calendar_sync`, `reach_out_detection`, `cadence_overdue`, the purge jobs, `alert_eval`, … — writes
one `job_runs` row: `job_name`, `trigger` (`scheduled` cron tick / `initial` boot catch-up /
`manual` `/admin/trigger-*`), `started_at` / `finished_at`, `duration_ms`, `result`
(`success` / `failure` / `skipped`), the sanitized `error` on the failure path, an optional
`items_processed` count (reminders sent, suggestions created), and the run's `correlation_id`.
`skipped` means the run did not execute — the distributed job lock was held or it ran too recently —
so a suppressed run is *recorded*, not silent.

`GET /admin/job-runs` lists the history newest-first, filterable by `job_name` / `result` /
`since` / `until` / `limit` (default 100, max 500). `GET /admin/job-runs/health` folds it per job:
`status` (`healthy` / `failing` / `unknown`), `last_run_at` / `last_result`, `last_success_at` /
`last_failure_at`, `consecutive_failures` + `incident_first_failure_at`, and an
`avg_duration_ms` / `max_duration_ms` trend over the last 20 executed runs — so "this job normally
takes 1 s but the last run took 40 s" is visible as a trend, not inferred from one row. `skipped`
runs are transparent to `status` and the failure streak. Like the surfaces above it is **derived on
read**; the only write path for `job_runs` is `RecordJobRun` from the job wrapper.

A `daily_reminders` run whose notification send fails now records `result: failure` with the send
error — the reminder job no longer reports success while a birthday reminder silently fails to send.

**Retention**: `JOB_RUN_RETENTION_DAYS` (default `30`). A daily purge job (`job_run_purge`)
hard-deletes rows whose `started_at` is older than the window; `<=0` disables it. Rendered as the
**Background jobs** panel at `/system-events` (web) and on the System Events screen (Android).

Consumers: `/metrics` ([#389](https://github.com/DrewBrunning/mycorrhizal-crm/issues/389)) will
export the per-job-outcome counters — those counters are `#389`'s scope, the outcome *records* are
this one's.

## Operational error aggregation

`GET /admin/error-aggregation` (admin-only) rolls the *failure* rows of `system_events` over a
rolling window (`?window_hours=`, default `24`, 1–168) up into one bucket **per cause** (issue
[#426](https://github.com/DrewBrunning/mycorrhizal-crm/issues/426)) — so an operator sees "CardDAV
authentication — 17" instead of 17 separate rows to correlate by hand.

A bucket's key is `(component, normalized error string)`. The normalization masks the parts of an
error that vary run to run — numbers and HTTP status codes, UUIDs, hex, IPs, `host:port`, URLs,
RFC3339 timestamps, quoted substrings, unix paths — so `carddav auth rejected … subscription 4821
(HTTP 401)` and `… 9137 (HTTP 403)` collapse to one bucket while `database is locked (SQLITE_BUSY)`
stays its own (the ordered substitution list is `causeMasks` in
`backend/services/error_aggregation.go`; it is unit-tested against the examples above).

Each bucket carries `count`, `recurring` (`count >= 3` — a single transient failure is `false`, a
repeating cause is `true` and shown prominently), `first_seen` / `last_seen`, `event_types`, a
`sample_error` (the most recent *raw* string, so a real instance is still visible), and `event_ids`
— the exact `system_events` rows behind it (capped at 500). Passing those to
`GET /admin/system-events?ids=<comma-separated>` opens the underlying events in the timeline; that is
how the web / Android "View N events" action links a bucket to its history.

Like per-subsystem health this is **derived on read** by folding `system_events` — no table, no
retention of its own, survives a restart. A failure event with no `error` string carries no cause
and is skipped (the only known case is the restore-drill row-count-mismatch path, which sets
`detail` not `error`). Rendered at `/system-events` (web) and Settings → System events (Android),
between the subsystem-health panel and the timeline.

## Alerting on state transitions

A scheduled evaluator (`alert_eval`, every `ALERT_EVAL_INTERVAL_MINUTES`, job-lock guarded,
config-gated by `ALERTING_ENABLED`; issue
[#428](https://github.com/DrewBrunning/mycorrhizal-crm/issues/428)) watches the per-subsystem
health above plus a few threshold checks and dispatches **exactly one notification per state
transition** — a failure *and* its recovery. It replaces the two fire-on-every-failed-run webhooks
(`db.integrity_check_failed`, `db.restore_drill_failed`) that had no recovery counterpart and no
de-duplication.

**How it avoids alert storms**: the current per-condition state (`ok` / `alerting`, and since
when) is persisted in `alert_states`, one row per condition. A dispatch happens only when the
freshly-computed verdict differs from the stored one, so a condition that keeps failing produces
one alert, not one per evaluation.

**Conditions** (each individually configurable; a `0` threshold disables that condition):

| condition | fires when | recovers when |
|---|---|---|
| `backup` | the `backup` subsystem is `failing` | it reports a success again (`backup_completed` / `restore_test_completed`) |
| `backup_stale` | backups have succeeded before but not within `ALERT_BACKUP_MAX_AGE_HOURS` (default `2 ×` the restore-drill interval) | a success lands inside the window |
| `sync:contact_sync` / `sync:calendar_sync` | the subsystem is `failing` with ≥ `ALERT_SYNC_FAILURE_THRESHOLD` consecutive failures | it reports healthy |
| `notifications` | the `notification` subsystem is `failing` with ≥ `ALERT_NOTIFY_FAILURE_THRESHOLD` consecutive failures | it reports healthy |
| `integrations` | the `webhook` subsystem is `failing` and its last failure is within `ALERT_INCIDENT_QUIET_HOURS` | no new `integration_failed` for that window (the webhook subsystem emits no success token — [#422](https://github.com/DrewBrunning/mycorrhizal-crm/issues/422)) |
| `db_integrity` | the last scheduled `PRAGMA integrity_check` result is `failed` / `error` | it flips back to `ok` |
| `disk_space` | the filesystem holding the DB is ≥ `ALERT_DISK_USAGE_PERCENT` full | usage drops 5 points below the threshold (hysteresis) |
| `job_stopped` | any config-enabled scheduled job's last **successful** completion is older than its interval × `ALERT_JOB_STALE_MULTIPLIER` | every watched job is fresh again |

**Delivery** reuses the existing paths:

- **Webhooks** — `alert.raised` / `alert.cleared`, broadcast to every subscriber (like the current
  `db.*` operator events). One event pair; the specific condition, state, detail, failure count and
  `since` timestamp are in the payload. Subscribe under Settings → Webhooks.
- **Personal channels** (email / ntfy / Gotify / push) — sent to **admin users only** (`is_admin`),
  through each admin's own notification config. Infra health is an operator concern, not something
  to page every user of a shared instance about.

Subjects follow `🔴 Backup failed` … `🟢 Backup recovered after 3 failures`.

## Related knobs

| env var | default | effect |
|---|---|---|
| `LOG_LEVEL` | `info` | `debug` \| `info` \| `warn` \| `error` |
| `LOG_PRETTY` | off in `release` | human-readable console output instead of JSON (dev) |
| `SYSTEM_EVENT_RETENTION_DAYS` | `30` | how long `system_events` rows survive |
| `WEBHOOK_DELIVERY_RETENTION_DAYS` | `30` | how long `webhook_deliveries` rows survive |
| `STORAGE_WARN_PERCENT` / `STORAGE_CRITICAL_PERCENT` | `75` / `90` | the `/admin/system-status` storage block's `ok`→`warning`→`critical` tiers, with -5% hysteresis; a warning/critical tier elevates `overall` to at least `degraded` (issue #652) |
| `STORAGE_SAMPLE_RETENTION_DAYS` | `180` | how long the daily `storage_samples` rows (the storage-growth trend history) survive before the sampler prunes them |
| `DB_INTEGRITY_CHECK_ENABLED` / `_INTERVAL_HOURS` | on / `24` | scheduled `PRAGMA integrity_check` |
| `DB_RESTORE_DRILL_ENABLED` / `_INTERVAL_HOURS` | on / `168` | scheduled backup-restore drill |
| `ALERTING_ENABLED` | on | master switch for the alert evaluator |
| `ALERT_EVAL_INTERVAL_MINUTES` | `15` | how often conditions are re-evaluated |
| `ALERT_DISK_USAGE_PERCENT` | `90` | `disk_space` threshold; `0` disables the condition |
| `ALERT_SYNC_FAILURE_THRESHOLD` / `ALERT_NOTIFY_FAILURE_THRESHOLD` | `3` / `3` | consecutive failures before `sync:*` / `notifications` fire |
| `ALERT_BACKUP_MAX_AGE_HOURS` | `0` → `2 ×` restore-drill interval | `backup_stale` threshold |
| `ALERT_JOB_STALE_MULTIPLIER` | `3` | `job_stopped` fires at interval × this |
| `ALERT_INCIDENT_QUIET_HOURS` | `6` | `integrations` recovery window |
| `ALERT_BACKUP_ENABLED` / `ALERT_DB_INTEGRITY_ENABLED` / `ALERT_JOB_STOPPED_ENABLED` | on | per-condition switches for the conditions with no numeric knob |
