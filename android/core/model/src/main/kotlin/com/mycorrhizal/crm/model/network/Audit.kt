package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * Audit operation tokens mirroring `backend/models/audit.go`'s AuditOp*
 * constants (T18). Hardcoded mirror of the backend — no dynamic type-list
 * endpoint exists anywhere in this codebase; keep in sync by hand if the
 * backend ever adds an operation.
 */
object AuditOperations {
    const val CREATE = "create"
    const val UPDATE = "update"
    const val DELETE = "delete"
}

/**
 * Audit entity-type tokens mirroring `backend/models/audit.go`'s AuditEntity*
 * constants exactly. [ALL] drives both the filter dropdown and the row labels.
 * Hardcoded mirror (frontend trap #4) — keep in sync by hand if the backend
 * ever adds an audited entity.
 */
object AuditEntityTypes {
    const val CONTACT = "contact"
    const val NOTE = "note"
    const val ACTIVITY = "activity"
    const val LIFE_EVENT = "life_event"
    const val GIFT = "gift"
    const val CIRCLE = "circle"
    const val TAG = "tag"
    const val HOUSEHOLD = "household"
    const val REMINDER = "reminder"

    /** Every entity token, in the backend's own declaration order. */
    val ALL: List<String> = listOf(
        CONTACT, NOTE, ACTIVITY, LIFE_EVENT, GIFT, CIRCLE, TAG, HOUSEHOLD, REMINDER,
    )
}

/**
 * One immutable create/update/delete record from the T18 audit log
 * (`GET /audit`). Reverse-chronological. `entity_id` is a Contact.VCardUID
 * for contact events (resolved to a display name/ID via `?vcard_uid=`), a
 * numeric-id string for every other entity.
 *
 * `before_snapshot` is opaque, redacted infrastructure JSON the UI must never
 * render as user-facing content — it exists so undo can restore the before
 * state server-side.
 */
@JsonClass(generateAdapter = true)
data class AuditEvent(
    val id: Long = 0,
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "entity_type") val entityType: String = "",
    @Json(name = "entity_id") val entityId: String = "",
    val operation: String = "",
    @Json(name = "before_snapshot") val beforeSnapshot: String? = null,
) {
    /** Mirrors web's undo gate: only contact update events can be undone (400 otherwise). */
    val canUndo: Boolean
        get() = entityType == AuditEntityTypes.CONTACT && operation == AuditOperations.UPDATE
}

/** GET /audit response — `{ audit_events, total }`. `total` counts this window, not all rows. */
@JsonClass(generateAdapter = true)
data class AuditEventsResponse(
    @Json(name = "audit_events") val auditEvents: List<AuditEvent> = emptyList(),
    val total: Int = 0,
)

/** POST /audit/:id/undo response — `{ message }` (the message is not surfaced). */
@JsonClass(generateAdapter = true)
data class AuditUndoResponse(
    val message: String? = null,
)
