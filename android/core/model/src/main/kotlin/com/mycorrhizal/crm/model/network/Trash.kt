package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

// Contact trash wire models (issue #2002, ADR 0037), mirroring
// `backend/models/trash_dto.go`.

/** One trashed (restorable) contact from `GET /trash/contacts`. */
@JsonClass(generateAdapter = true)
data class TrashedContact(
    val contact: ContactSummary = ContactSummary(),
    @Json(name = "deleted_at") val deletedAt: String? = null,
    /** When the scheduled purge hard-deletes it; null when retention is 0 (keep forever). */
    @Json(name = "purge_at") val purgeAt: String? = null
)

@JsonClass(generateAdapter = true)
data class TrashListResponse(
    val contacts: List<TrashedContact> = emptyList(),
    val total: Int = 0,
    val page: Int = 1,
    val limit: Int = 25,
    /** DELETED_RETENTION_DAYS; 0 means trashed contacts are kept forever. */
    @Json(name = "retention_days") val retentionDays: Int = 0
)

/** Rows a restore could not bring back (their other side is gone, or the key was re-created). */
@JsonClass(generateAdapter = true)
data class TrashRestoreSkip(val table: String = "", val reason: String = "", val count: Int = 0)

@JsonClass(generateAdapter = true)
data class TrashRestoreResponse(
    val contact: ContactSummary = ContactSummary(),
    val skipped: List<TrashRestoreSkip> = emptyList()
)

@JsonClass(generateAdapter = true)
data class TrashPurgeResponse(val message: String = "", val purged: Int = 0)

/**
 * One trashed (restorable) entity from `GET /trash/entities` (issue #2446, mirroring
 * `backend/models/trash_dto.go`). Entities have no purge endpoint; they are restored only.
 */
@JsonClass(generateAdapter = true)
data class TrashedEntity(
    val entity: Entity = Entity(),
    @Json(name = "deleted_at") val deletedAt: String? = null,
    /** When the scheduled purge hard-deletes it; null when retention is 0 (keep forever). */
    @Json(name = "purge_at") val purgeAt: String? = null,
    /** The survivor's id when this entity is a merge loser; absent for a plain delete. */
    @Json(name = "merged_into") val mergedInto: String? = null,
    /** The survivor's name; absent when the survivor is gone. */
    @Json(name = "merged_into_name") val mergedIntoName: String? = null
)

@JsonClass(generateAdapter = true)
data class TrashEntityListResponse(
    // Nullable on the wire like every collection (trap #8).
    val entities: List<TrashedEntity>? = null,
    val total: Int = 0,
    val page: Int = 1,
    val limit: Int = 25,
    @Json(name = "retention_days") val retentionDays: Int = 0
) {
    val entitiesOrEmpty: List<TrashedEntity> get() = entities.orEmpty()
}
