package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * Mirrors backend/models/contact_share.go's ContactShareStatus* constants and
 * the `oneof=pending accepted declined` validator (frontend trap 4 — hardcoded
 * mirror, kept in sync by hand).
 */
object ContactShareStatuses {
    const val PENDING = "pending"
    const val ACCEPTED = "accepted"
    const val DECLINED = "declined"
    val ALL: List<String> = listOf(PENDING, ACCEPTED, DECLINED)
}

/**
 * A one-time, filtered copy of a contact offered from one user to another on
 * the same instance (P1). The
 * payload is a frozen JSContact snapshot — editing the original contact after
 * sharing has no effect. UUID-string PK.
 */
@JsonClass(generateAdapter = true)
data class ContactShare(
    val id: String = "",
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "updated_at") val updatedAt: String? = null,
    @Json(name = "from_user_id") val fromUserId: Long = 0,
    @Json(name = "to_user_id") val toUserId: Long = 0,
    @Json(name = "contact_display_name") val contactDisplayName: String = "",
    val status: String = ContactShareStatuses.PENDING,
    @Json(name = "responded_at") val respondedAt: String? = null,
)

/** GET /contact-shares/incoming|outgoing response — cursor-paginated. */
@JsonClass(generateAdapter = true)
data class ContactSharesPage(
    @Json(name = "contact_shares") val contactShares: List<ContactShare> = emptyList(),
    /** Keyed by the OTHER party's user ID (stringified) -> their username. */
    val usernames: Map<String, String> = emptyMap(),
    val total: Int = 0,
    @Json(name = "next_cursor") val nextCursor: String? = null,
    val limit: Int = 0,
)

/** POST /contact-shares response — wrapped `{ message, contact_share }`. */
@JsonClass(generateAdapter = true)
data class CreateContactShareResponse(
    val message: String? = null,
    @Json(name = "contact_share") val contactShare: ContactShare? = null,
)

/** POST /contact-shares body. */
@JsonClass(generateAdapter = true)
data class ContactShareInput(
    @Json(name = "to_user_id") val toUserId: Long,
    @Json(name = "vcard_uid") val vcardUid: String,
    val sections: List<String>,
    @Json(name = "include_sensitive") val includeSensitive: Boolean = false,
)

/**
 * Thin per-user shape GET /users/directory returns — id + username only, for
 * any authenticated user (unlike the admin-only full user list).
 */
@JsonClass(generateAdapter = true)
data class UserDirectoryEntry(
    val id: Long = 0,
    val username: String = "",
)

/** GET /users/directory response — `{ users: [...] }`. */
@JsonClass(generateAdapter = true)
data class UserDirectoryResponse(
    val users: List<UserDirectoryEntry> = emptyList(),
)
