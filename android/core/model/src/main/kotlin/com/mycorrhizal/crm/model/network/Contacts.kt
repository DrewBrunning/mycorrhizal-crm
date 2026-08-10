package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * Slim per-item shape for GET /contacts (list) — matches the OpenAPI
 * ContactSummary schema. The list query does not select the `circles`
 * column, so it is typically null; do not rely on it being populated.
 */
@JsonClass(generateAdapter = true)
data class ContactSummary(
    val id: Int = 0,
    val uid: String? = null,
    val firstname: String? = null,
    val lastname: String? = null,
    val nickname: String? = null,
    /** Full/display name (vCard FN). */
    val fn: String? = null,
    @Json(name = "primary_email") val primaryEmail: String? = null,
    @Json(name = "primary_phone") val primaryPhone: String? = null,
    val birthday: String? = null,
    val org: String? = null,
    val photo: String? = null,
    @Json(name = "photo_thumbnail") val photoThumbnail: String? = null,
    val circles: List<String>? = null,
    val archived: Boolean = false,
    /** Change-feed tombstone marker (T17): true only via ?since=. */
    val deleted: Boolean = false,
) {
    val displayName: String
        get() = fn?.takeIf { it.isNotBlank() }
            ?: listOfNotNull(firstname, lastname).joinToString(" ").ifBlank { "#$id" }
}

/** GET /contacts response envelope (T17 cursor pagination). */
@JsonClass(generateAdapter = true)
data class ContactsPage(
    val contacts: List<ContactSummary> = emptyList(),
    @Json(name = "next_cursor") val nextCursor: String? = null,
    val limit: Int = 0,
    val sync: SyncInfo? = null,
)

/**
 * T17 sync-mode contract carried by every cursor-paginated list response.
 * `mode` is how THIS collection syncs; the arrays are the static map of
 * every syncable collection.
 */
@JsonClass(generateAdapter = true)
data class SyncInfo(
    val mode: String? = null,
    val incremental: List<String>? = null,
    @Json(name = "full_resync") val fullResync: List<String>? = null,
)
