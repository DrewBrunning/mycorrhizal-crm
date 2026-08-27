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
