package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * System-event type tokens mirroring `backend/models/system_event.go`'s
 * SysEvent* constants, migration 000038's CHECK constraint, and web's
 * `frontend/src/api/systemEvents.ts` exactly. Hardcoded mirror — no dynamic
 * type-list endpoint exists (frontend trap #4); keep the four in sync by hand.
 */
object SystemEventTypes {
    const val APPLICATION_STARTED = "application_started"
    const val APPLICATION_STOPPED = "application_stopped"
    const val MIGRATION_STARTED = "migration_started"
    const val MIGRATION_COMPLETED = "migration_completed"
    const val MIGRATION_FAILED = "migration_failed"
    const val JOB_STARTED = "job_started"
    const val JOB_COMPLETED = "job_completed"
    const val JOB_FAILED = "job_failed"
    const val SYNC_STARTED = "sync_started"
    const val SYNC_COMPLETED = "sync_completed"
    const val SYNC_FAILED = "sync_failed"
    const val NOTIFICATION_SENT = "notification_sent"
    const val NOTIFICATION_FAILED = "notification_failed"
    const val BACKUP_COMPLETED = "backup_completed"
    const val BACKUP_FAILED = "backup_failed"
    const val RESTORE_TEST_COMPLETED = "restore_test_completed"
    const val INTEGRATION_FAILED = "integration_failed"

    /** Every token, in the backend's declaration order. */
    val ALL: List<String> = listOf(
        APPLICATION_STARTED, APPLICATION_STOPPED,
        MIGRATION_STARTED, MIGRATION_COMPLETED, MIGRATION_FAILED,
        JOB_STARTED, JOB_COMPLETED, JOB_FAILED,
        SYNC_STARTED, SYNC_COMPLETED, SYNC_FAILED,
        NOTIFICATION_SENT, NOTIFICATION_FAILED,
        BACKUP_COMPLETED, BACKUP_FAILED, RESTORE_TEST_COMPLETED,
        INTEGRATION_FAILED,
    )
}

/** Severity tokens (`backend/logger/fields.go` Severity*). */
object SystemEventSeverities {
    const val INFO = "info"
    const val WARN = "warn"
    const val ERROR = "error"

    val ALL: List<String> = listOf(INFO, WARN, ERROR)
}

/**
 * The component values the backend producers emit today
 * (`backend/logger/fields.go` Component*). `component` is a free-form string
 * server-side, so this list only seeds the filter dropdown — an unknown value
 * from a future producer still renders.
 */
object SystemEventComponents {
    val ALL: List<String> = listOf(
        "app", "scheduler", "migration",
        "contact_sync", "calendar_sync",
        "notification", "webhook", "backup",
    )
}

/**
 * One persisted operational event from `GET /admin/system-events` (issue
 * #424). Reverse-chronological by [occurredAt]. System-generated, read-only,
 * admin-only. `correlationId` ties the event to its chain of work — an HTTP
 * request id, or `job:<name>:<uuid>`.
 */
@JsonClass(generateAdapter = true)
data class SystemEvent(
    val id: Long = 0,
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "occurred_at") val occurredAt: String = "",
    @Json(name = "event_type") val eventType: String = "",
    val severity: String = "",
    val component: String = "",
    val operation: String? = null,
    @Json(name = "duration_ms") val durationMs: Long? = null,
    val result: String? = null,
    @Json(name = "correlation_id") val correlationId: String = "",
    val error: String? = null,
    val detail: String? = null,
    @Json(name = "user_id") val userId: Long? = null,
)

/** GET /admin/system-events response — `{ system_events, total }`. */
@JsonClass(generateAdapter = true)
data class SystemEventsResponse(
    @Json(name = "system_events") val systemEvents: List<SystemEvent> = emptyList(),
    val total: Int = 0,
)

/**
 * The subsystems tracked by `GET /admin/subsystem-health` (issue #427),
 * mirroring `backend/services/subsystem_health.go`'s subsystemDefs and
 * `backend/openapi.yaml`'s SubsystemHealth.subsystem enum EXACTLY, and in the
 * same order (the API preserves it). Hand-maintained mirror — no dynamic list
 * endpoint exists (frontend trap #4); keep web's `SUBSYSTEMS` in sync too.
 */
object Subsystems {
    const val CONTACT_SYNC = "contact_sync"
    const val CALENDAR_SYNC = "calendar_sync"
    const val NOTIFICATION = "notification"
    const val BACKUP = "backup"
    const val SCHEDULER = "scheduler"
    const val WEBHOOK = "webhook"

    val ALL: List<String> = listOf(
        CONTACT_SYNC, CALENDAR_SYNC, NOTIFICATION, BACKUP, SCHEDULER, WEBHOOK,
    )
}

/** Subsystem-health status tokens (`backend/services` SubsystemStatus*). */
object SubsystemStatuses {
    const val HEALTHY = "healthy"
    const val FAILING = "failing"
    const val UNKNOWN = "unknown"
}

/**
 * The last-known-good state of one operational subsystem from
 * `GET /admin/subsystem-health` (issue #427), derived on the server by folding
 * the operational-event stream. [status] is healthy / failing / unknown;
 * [incidentFirstFailureAt] is non-null exactly when [consecutiveFailures] > 0.
 * The scheduler and webhook subsystems emit only a failure event today, so
 * they never reach `healthy` until a success-side event exists (#422).
 */
@JsonClass(generateAdapter = true)
data class SubsystemHealth(
    val subsystem: String = "",
    val status: String = "",
    @Json(name = "last_attempt_at") val lastAttemptAt: String? = null,
    @Json(name = "last_success_at") val lastSuccessAt: String? = null,
    @Json(name = "last_failure_at") val lastFailureAt: String? = null,
    @Json(name = "incident_first_failure_at") val incidentFirstFailureAt: String? = null,
    @Json(name = "consecutive_failures") val consecutiveFailures: Int = 0,
    @Json(name = "last_error") val lastError: String = "",
)

/** GET /admin/subsystem-health response — `{ subsystems }`. */
@JsonClass(generateAdapter = true)
data class SubsystemHealthResponse(
    val subsystems: List<SubsystemHealth> = emptyList(),
)

/**
 * One aggregated operational-error cause from `GET /admin/error-aggregation`
 * (issue #426): every system_events failure row sharing a [component] and a
 * normalized error string, collapsed to one row with a [count]. [recurring] is
 * `count >= 3` — a single transient failure is not an alarm, a repeating cause
 * is. [eventIds] are the exact system_events rows behind the bucket (capped at
 * 500); pass them to `GET /admin/system-events?ids=` for the timeline
 * drill-down. [sampleError] is the most recent raw error string, kept so an
 * operator still sees a real instance.
 */
@JsonClass(generateAdapter = true)
data class ErrorBucket(
    val component: String = "",
    val cause: String = "",
    @Json(name = "sample_error") val sampleError: String = "",
    @Json(name = "event_types") val eventTypes: List<String> = emptyList(),
    val count: Int = 0,
    val recurring: Boolean = false,
    @Json(name = "first_seen") val firstSeen: String = "",
    @Json(name = "last_seen") val lastSeen: String = "",
    @Json(name = "event_ids") val eventIds: List<Long> = emptyList(),
    @Json(name = "event_ids_truncated") val eventIdsTruncated: Boolean = false,
)

/** GET /admin/error-aggregation response — `{ window_hours, since, until, total_events, buckets }`. */
@JsonClass(generateAdapter = true)
data class ErrorAggregationResponse(
    @Json(name = "window_hours") val windowHours: Int = 24,
    val since: String = "",
    val until: String = "",
    @Json(name = "total_events") val totalEvents: Int = 0,
    val buckets: List<ErrorBucket> = emptyList(),
)

/**
 * Background-job run result / trigger tokens (issue #391), mirroring
 * `backend/models/job_run.go`, migration 000041's CHECK constraint,
 * `backend/openapi.yaml`, and web's `frontend/src/api/jobRuns.ts` EXACTLY.
 * Hand-maintained mirror — no dynamic list endpoint exists (frontend trap #4).
 */
object JobRunResults {
    const val SUCCESS = "success"
    const val FAILURE = "failure"
    const val SKIPPED = "skipped"

    val ALL: List<String> = listOf(SUCCESS, FAILURE, SKIPPED)
}

object JobRunTriggers {
    const val SCHEDULED = "scheduled"
    const val INITIAL = "initial"
    const val MANUAL = "manual"

    val ALL: List<String> = listOf(SCHEDULED, INITIAL, MANUAL)
}

/** Job-run health status tokens (`backend/services` JobRunStatus*). */
object JobRunStatuses {
    const val HEALTHY = "healthy"
    const val FAILING = "failing"
    const val UNKNOWN = "unknown"
}

/**
 * One persisted background-job execution outcome from `GET /admin/job-runs`
 * (issue #391). System-generated, read-only, admin-only. [trigger] is
 * scheduled / initial / manual; [result] is success / failure / skipped
 * (skipped = the run did not execute — job lock held / ran too recently).
 */
@JsonClass(generateAdapter = true)
data class JobRun(
    val id: Long = 0,
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "job_name") val jobName: String = "",
    val trigger: String = "",
    @Json(name = "started_at") val startedAt: String = "",
    @Json(name = "finished_at") val finishedAt: String = "",
    @Json(name = "duration_ms") val durationMs: Long = 0,
    val result: String = "",
    val error: String? = null,
    @Json(name = "items_processed") val itemsProcessed: Long? = null,
    val detail: String? = null,
    @Json(name = "correlation_id") val correlationId: String = "",
)

/** GET /admin/job-runs response — `{ job_runs, total }`. */
@JsonClass(generateAdapter = true)
data class JobRunsResponse(
    @Json(name = "job_runs") val jobRuns: List<JobRun> = emptyList(),
    val total: Int = 0,
)

/**
 * The folded run health of one background job from `GET /admin/job-runs/health`
 * (issue #391), derived on the server from the job_runs history. [status] is
 * healthy / failing / unknown; [incidentFirstFailureAt] is non-null exactly
 * when [consecutiveFailures] > 0. [avgDurationMs] / [maxDurationMs] are the
 * trend over the last [durationSampleSize] executed runs.
 */
@JsonClass(generateAdapter = true)
data class JobRunHealth(
    @Json(name = "job_name") val jobName: String = "",
    val status: String = "",
    @Json(name = "last_run_at") val lastRunAt: String? = null,
    @Json(name = "last_result") val lastResult: String = "",
    @Json(name = "last_trigger") val lastTrigger: String = "",
    @Json(name = "last_duration_ms") val lastDurationMs: Long? = null,
    @Json(name = "last_items_processed") val lastItemsProcessed: Long? = null,
    @Json(name = "last_success_at") val lastSuccessAt: String? = null,
    @Json(name = "last_failure_at") val lastFailureAt: String? = null,
    @Json(name = "last_error") val lastError: String = "",
    @Json(name = "incident_first_failure_at") val incidentFirstFailureAt: String? = null,
    @Json(name = "consecutive_failures") val consecutiveFailures: Int = 0,
    @Json(name = "duration_sample_size") val durationSampleSize: Int = 0,
    @Json(name = "avg_duration_ms") val avgDurationMs: Long? = null,
    @Json(name = "max_duration_ms") val maxDurationMs: Long? = null,
)

/** GET /admin/job-runs/health response — `{ jobs }`. */
@JsonClass(generateAdapter = true)
data class JobRunHealthResponse(
    val jobs: List<JobRunHealth> = emptyList(),
)
