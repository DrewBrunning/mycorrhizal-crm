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
    // Issue #212: CRM-local favorite flag (web #173). The backend serializes
    // it unconditionally (no omitempty — it defaults to false), mirroring the
    // archived flag, so the star icon can always trust its presence.
    @Json(name = "is_favorite") val isFavorite: Boolean = false,
    /** Change-feed tombstone marker (T17): true only via ?since=. */
    val deleted: Boolean = false,
    /** Device Contacts LOOKUP_KEY after a T57 import (§7.5.4); null otherwise. */
    @Json(name = "device_lookup_key") val deviceLookupKey: String? = null
) {
    val displayName: String
        get() = contactDisplayName(firstname, nickname, lastname, fn, "#$id")
}

/**
 * The single display-name rule (M5 §3.2, issue #1762): `firstname "nickname"
 * lastname`, falling back to `fn` and then [fallbackId]. The dashboard and
 * the contact list must render the same person identically, so both call
 * this instead of keeping their own nickname-preferred variants.
 */
fun contactDisplayName(
    firstname: String?,
    nickname: String?,
    lastname: String?,
    fn: String? = null,
    fallbackId: String? = null
): String {
    val parts = listOfNotNull(
        firstname?.takeIf { it.isNotBlank() },
        nickname?.takeIf { it.isNotBlank() }?.let { "\"$it\"" },
        lastname?.takeIf { it.isNotBlank() }
    )
    if (parts.isNotEmpty()) return parts.joinToString(" ")
    return fn?.takeIf { it.isNotBlank() } ?: fallbackId.orEmpty()
}

/** GET /contacts response envelope (T17 cursor pagination). */
@JsonClass(generateAdapter = true)
data class ContactsPage(
    val contacts: List<ContactSummary> = emptyList(),
    @Json(name = "next_cursor") val nextCursor: String? = null,
    val limit: Int = 0,
    val sync: SyncInfo? = null
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
    @Json(name = "full_resync") val fullResync: List<String>? = null
)
