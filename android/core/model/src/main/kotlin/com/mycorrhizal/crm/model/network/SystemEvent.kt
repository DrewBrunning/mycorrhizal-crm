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
