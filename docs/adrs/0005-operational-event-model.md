# ADR 0005: Operational-event model, separate from the audit trail

- **Status:** accepted
- **Date:** 2026-08-27
- **Issues:** [#424](https://github.com/DrewBrunning/mycorrhizal-crm/issues/424) (structured
  operational-event model + system event timeline), [#425](https://github.com/DrewBrunning/mycorrhizal-crm/issues/425)
  (structured logging + correlation IDs)

## Context

Operational state — did the last backup drill pass, is a sync failing, when did the app restart —
was scattered across the audit trail (`audit_events`, which records *who changed user data*),
`job_executions` (lock + last-run only), delivery tables, sync-conflict rows, and ephemeral zerolog
lines that vanish on restart. An admin had no chronological, restart-surviving answer to "when did
this start / what changed / what's failing", which is the foundation the rest of the observability
work (error aggregation, last-known-good, alerting) needs.

Two shapes were candidates: extend the audit trail with system event types, or add a dedicated
table.

## Decision

**Operational events are a dedicated `system_events` table, distinct from `audit_events`.**

- **Separate from the audit trail.** `audit_events` answers "who changed user-authored data" and is a
  tamper-evident, hash-chained, 90-day investigation record scoped by `user_id`. `system_events`
  answers "what happened to the instance" — it is not user data, has no per-user scoping on the
  query, and is admin-only over the API. Overloading the audit trail would have forced system events
  into its hash chain and its retention window, and blurred a boundary the security docs rely on.
- **Hard delete, bounded retention.** System-generated, no `deleted_at`
  ([ADR 0004](0004-soft-vs-hard-delete-semantics.md)'s rule). Removed only by
  `PurgeExpiredSystemEvents` past `SYSTEM_EVENT_RETENTION_DAYS` (default **30** — deliberately shorter
  than audit's 90: operational noise is not an investigation record, and the window has to bound
  growth on a single-file SQLite database). `<=0` disables the purge rather than deleting everything,
  matching the audit purge.
- **No hash chain.** This is diagnostics, not a legal record. The tamper-evidence machinery
  (`RecomputeAuditChain`, the 000016 immutability trigger) is not applied.
- **No metrics.** Counters, gauges, and timings do not live here — that is
  [#389](https://github.com/DrewBrunning/mycorrhizal-crm/issues/389)'s concern. `system_events`
  stores discrete state transitions (`job_failed`, `sync_completed`, `backup_failed`, …) with a
  small, `CHECK`-constrained type vocabulary; `duration_ms` is carried on the event that already
  exists, not as a standalone series.
- **Low cardinality only.** No contact IDs, no raw URLs, no per-entity identifiers. The `detail`
  field is for bounded values (counts, subsystem names, a from/to version); `error` is sanitized and
  length-capped by `models.RecordSystemEvent`.
- **Correlation IDs tie it together.** Every event carries a `correlation_id` — the originating HTTP
  request's ID, or `job:<name>:<uuid>` for a scheduled run — so the timeline UI can pivot from one
  event to every other event in the same chain of work, and an operator can grep the same ID across
  the stdout log. The propagation plumbing (`logger.WithCorrelationID` / `logger.Ctx`, the request-ID
  middleware, the scheduler `runJob` wrapper, the outbound `X-Correlation-ID` header) is #425's half
  of this ADR.
- **Best-effort emission.** `RecordSystemEvent` never returns an error and never blocks the caller —
  a diagnostic write must not be able to fail the operation it describes.

### Which producers emit events

The operational producers whose failures an operator needs on the timeline: the scheduler
(`job_failed` on a recovered panic), contact/calendar sync (`sync_completed` / `sync_failed`), the
restore drill (`restore_test_completed` / `backup_failed`), the migration runner
(`migration_completed`), notification dispatch (`notification_sent` / `notification_failed`, per
channel), webhook delivery (`integration_failed` once a delivery exhausts its retry budget), and the
process itself (`application_started` / `application_stopped`).

Notification and webhook delivery keep their per-delivery *detail* tables
(`notification_deliveries`, `webhook_deliveries`) — the `system_events` row is the timeline summary,
not a replacement; per-channel delivery *health* (rates, last-good) is still
[#422](https://github.com/DrewBrunning/mycorrhizal-crm/issues/422)'s concern. The ntfy/Gotify/FCM
push channels do not yet set an outbound correlation header (their protocols have no standard one);
the emitted events and log lines carry the correlation ID.

## Consequences

- A new persistent copy of instance data exists. It is recorded in
  `docs/security/data-retention-lifecycle.md` (§3) and
  `docs/security/asvs-l2-verification-report.md` §9.
- Adding a new operational producer means adding a `RecordSystemEvent` call at its state transitions
  and, if it is a new *kind* of event, a token in `models/system_event.go`, migration `000038`'s
  `CHECK` list, `frontend/src/api/systemEvents.ts`, and the Android `SystemEventTypes` mirror — the
  same hand-sync discipline as the audit vocabulary (CLAUDE.md frontend trap #4).
- Raw-log storage/search remains the operator's stdout log driver (self-hosted, single process). The
  timeline's "view related" is a pivot across `system_events` by correlation ID, not an in-app log
  browser; `docs/operations/observability.md` says so explicitly.
- The per-subsystem last-known-good rollup ([#427](https://github.com/DrewBrunning/mycorrhizal-crm/issues/427),
  the "last-known-good" foundation this ADR's Context anticipates) is a **read-side fold** over this
  stream — `services.ComputeSubsystemHealth`, surfaced at `GET /admin/subsystem-health`. No second
  write path, no stored state: it recomputes from `system_events` on every read, so it survives a
  restart and never drifts. `/metrics` (#389) and transition alerting (#428) consume that rollup
  rather than each deriving their own.
- Error aggregation ([#426](https://github.com/DrewBrunning/mycorrhizal-crm/issues/426),
  `services.AggregateOperationalErrors`, `GET /admin/error-aggregation`) is a **sibling read-side
  fold** over the same stream — not a consumer of `ComputeSubsystemHealth`. It buckets the failure
  rows in a rolling window by `(component, normalized error string)` so a recurring cause collapses
  to one counted row, and adds no persistent data of its own. `GET /admin/system-events?ids=` is the
  exact-row drill-down from a bucket to its underlying events.
