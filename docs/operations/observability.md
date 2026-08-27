# Observability: logs, correlation IDs, and the system-event timeline

Where to look when something is failing in production, and how the pieces connect. Companion to
`docs/deployment.md` (install/backup/restore) and `docs/security/data-retention-lifecycle.md` (how
long each of these records survives).

| | |
|---|---|
| **Last updated** | 2026-08-27 (issues [#424](https://github.com/DrewBrunning/mycorrhizal-crm/issues/424), [#425](https://github.com/DrewBrunning/mycorrhizal-crm/issues/425)) |
| **Audience** | Operators diagnosing a failure without local reproduction. |
| **ADR** | [`docs/adrs/0005-operational-event-model.md`](../adrs/0005-operational-event-model.md) |

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
`000037`): `application_started`/`application_stopped`, `migration_started`/`migration_completed`/
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

Notification and webhook delivery keep their own per-delivery records
(`notification_deliveries`, `webhook_deliveries`) and their own follow-up
([#422](https://github.com/DrewBrunning/mycorrhizal-crm/issues/422)); they are not duplicated here.

**Retention**: `SYSTEM_EVENT_RETENTION_DAYS` (default `30`). A daily purge job
(`system_event_purge`) hard-deletes rows older than the window; `<=0` disables the purge (it never
deletes everything). Rows are present in a backup and are only as fresh as the snapshot.

## Related knobs

| env var | default | effect |
|---|---|---|
| `LOG_LEVEL` | `info` | `debug` \| `info` \| `warn` \| `error` |
| `LOG_PRETTY` | off in `release` | human-readable console output instead of JSON (dev) |
| `SYSTEM_EVENT_RETENTION_DAYS` | `30` | how long `system_events` rows survive |
| `DB_INTEGRITY_CHECK_ENABLED` / `_INTERVAL_HOURS` | on / `24` | scheduled `PRAGMA integrity_check` |
| `DB_RESTORE_DRILL_ENABLED` / `_INTERVAL_HOURS` | on / `168` | scheduled backup-restore drill |
