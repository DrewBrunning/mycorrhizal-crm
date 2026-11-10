package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * Mirrors backend/models/share_grant.go's ShareGrantStatus* constants (ADR 0042 decisions 7-8;
 * frontend trap 4 — a hardcoded mirror kept in sync by hand). The server's internal `blocked`
 * status is never on the wire: the sender sees it as `pending`.
 */
object ShareGrantStatuses {
    const val PENDING = "pending"
    const val ACTIVE = "active"
    const val DECLINED = "declined"
    const val REVOKED = "revoked"
    const val EXPIRED = "expired"
}

/** Mirrors the `entity_type` CHECK on share_grants (ADR 0042 decision 4). */
object ShareGrantEntityTypes {
    const val CONTACT = "contact"
    const val NOTE = "note"
    const val ACTIVITY = "activity"
    const val CIRCLE = "circle"
}

/** Mirrors the closed `revoked_reason` set (ADR 0042 decision 10). */
object ShareGrantRevokedReasons {
    const val OWNER_REVOKED = "owner_revoked"
    const val ENTITY_DELETED = "entity_deleted"
    const val RECIPIENT_DELETED = "recipient_deleted"
}

/**
 * A grant as its sender sees it (`ShareGrantOwnerView`). It stores authorization, never content.
 * [label] is computed per request by the server; [memberCount] is only set for a circle grant.
 */
@JsonClass(generateAdapter = true)
data class OutgoingShareGrant(
    val id: String = "",
    @Json(name = "to_user_id") val toUserId: Long = 0,
    @Json(name = "to_username") val toUsername: String = "",
    @Json(name = "entity_type") val entityType: String = "",
    @Json(name = "entity_id") val entityId: String = "",
    val label: String = "",
    val sections: List<String> = emptyList(),
    @Json(name = "reconfirm_sections") val reconfirmSections: List<String> = emptyList(),
    val status: String = ShareGrantStatuses.PENDING,
    @Json(name = "expires_at") val expiresAt: String? = null,
    @Json(name = "responded_at") val respondedAt: String? = null,
    @Json(name = "revoked_at") val revokedAt: String? = null,
    @Json(name = "revoked_reason") val revokedReason: String? = null,
    @Json(name = "member_count") val memberCount: Int? = null,
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "updated_at") val updatedAt: String? = null,
    /** ADR 0008: the value an `If-Match` precondition on PATCH echoes. */
    val revision: Int = 0
)

/**
 * A grant as its recipient sees it (`ShareGrantRecipientView`): the sender, the entity type and a
 * short label — never content. A pending row shows only this.
 */
@JsonClass(generateAdapter = true)
data class IncomingShareGrant(
    val id: String = "",
    @Json(name = "from_username") val fromUsername: String = "",
    @Json(name = "entity_type") val entityType: String = "",
    val label: String = "",
    val sections: List<String> = emptyList(),
    val status: String = ShareGrantStatuses.PENDING,
    @Json(name = "expires_at") val expiresAt: String? = null,
    @Json(name = "responded_at") val respondedAt: String? = null,
    @Json(name = "created_at") val createdAt: String? = null
)

/** GET /share-grants/outgoing — cursor-paginated, deliberately without a total. */
@JsonClass(generateAdapter = true)
data class OutgoingShareGrantsPage(
    @Json(name = "share_grants") val shareGrants: List<OutgoingShareGrant> = emptyList(),
    @Json(name = "next_cursor") val nextCursor: String? = null,
    val limit: Int = 0
)

/** GET /share-grants/incoming — pending and active only. */
@JsonClass(generateAdapter = true)
data class IncomingShareGrantsPage(
    @Json(name = "share_grants") val shareGrants: List<IncomingShareGrant> = emptyList(),
    @Json(name = "next_cursor") val nextCursor: String? = null,
    val limit: Int = 0
)

/** `{ share_grant }` wrapper of the owner-side create/patch/revoke/reconfirm responses. */
@JsonClass(generateAdapter = true)
data class OutgoingShareGrantResponse(
    @Json(name = "share_grant") val shareGrant: OutgoingShareGrant? = null
)

/** `{ share_grant }` wrapper of the recipient-side accept/decline/leave responses. */
@JsonClass(generateAdapter = true)
data class IncomingShareGrantResponse(
    @Json(name = "share_grant") val shareGrant: IncomingShareGrant? = null
)

/**
 * POST /share-grants body (ADR 0042 decision 13): the only things a client names. There is
 * deliberately no include-sensitive switch (decision 6).
 */
@JsonClass(generateAdapter = true)
data class ShareGrantInput(
    @Json(name = "to_user_id") val toUserId: Long,
    @Json(name = "entity_type") val entityType: String,
    @Json(name = "entity_id") val entityId: String,
    val sections: List<String> = emptyList(),
    @Json(name = "expires_at") val expiresAt: String? = null
)

/**
 * PATCH /share-grants/{id}: only `expires_at` and a narrowing `sections` list. [clearExpiry]
 * sends an explicit `"expires_at": null`; with neither [expiresAt] nor [clearExpiry] the field is
 * omitted so the server leaves it alone.
 */
data class ShareGrantPatch(
    val expiresAt: String? = null,
    val clearExpiry: Boolean = false,
    val sections: List<String>? = null
)

/** A sender this user has blocked (`ShareBlockView`). */
@JsonClass(generateAdapter = true)
data class ShareBlock(
    @Json(name = "user_id") val userId: Long = 0,
    val username: String = "",
    @Json(name = "created_at") val createdAt: String? = null
)

/** GET /share-grants/blocks response. */
@JsonClass(generateAdapter = true)
data class ShareBlocksResponse(val blocks: List<ShareBlock> = emptyList())

/** POST /share-grants/{id}/decline body. */
@JsonClass(generateAdapter = true)
data class ShareGrantDeclineInput(val block: Boolean = false)

/** GET /share-grants/notifications: the pending badge plus the newest unread notices. */
@JsonClass(generateAdapter = true)
data class ShareGrantNotifications(
    @Json(name = "pending_count") val pendingCount: Int = 0,
    val notifications: List<ShareGrantNotice> = emptyList()
)

/** One in-app notice: sender username and entity type only (ADR 0042 decision 11). */
@JsonClass(generateAdapter = true)
data class ShareGrantNotice(
    val id: Long = 0,
    val kind: String = "",
    @Json(name = "entity_type") val entityType: String = "",
    @Json(name = "sender_username") val senderUsername: String = "",
    @Json(name = "created_at") val createdAt: String? = null
)

/** A granted note's allowlisted fields: text and calendar date, never its contact link. */
@JsonClass(generateAdapter = true)
data class SharedNote(val content: String = "", val date: String? = null)

/** A granted activity's allowlisted fields. */
@JsonClass(generateAdapter = true)
data class SharedActivity(
    val title: String = "",
    val description: String = "",
    val location: String = "",
    val date: String? = null,
    @Json(name = "end_date") val endDate: String? = null,
    val type: String = ""
)

/** One live circle member: a display name only. */
@JsonClass(generateAdapter = true)
data class SharedCircleMember(@Json(name = "display_name") val displayName: String = "")

/** A granted circle: metadata plus live member names, capped by the server. */
@JsonClass(generateAdapter = true)
data class SharedCircle(
    val name: String = "",
    val description: String = "",
    val color: String = "",
    @Json(name = "member_count") val memberCount: Int = 0,
    val members: List<SharedCircleMember> = emptyList(),
    @Json(name = "members_truncated") val membersTruncated: Boolean = false
)

/**
 * GET /share-grants/{id}/content — the one recipient read route. Exactly one of
 * [contact]/[note]/[activity]/[circle] is set. [contact] is the server's JSContact projection
 * kept as a generic map: the client renders only a fixed allowlist of its properties
 * (`SharedContactProjection`), so an unexpected key is never shown.
 */
@JsonClass(generateAdapter = true)
data class ShareGrantContent(
    @Json(name = "grant_id") val grantId: String = "",
    @Json(name = "entity_type") val entityType: String = "",
    @Json(name = "owner_username") val ownerUsername: String = "",
    val contact: Map<String, Any?>? = null,
    val note: SharedNote? = null,
    val activity: SharedActivity? = null,
    val circle: SharedCircle? = null
)

/** `{ share_grant_content }` wrapper. */
@JsonClass(generateAdapter = true)
data class ShareGrantContentResponse(
    @Json(name = "share_grant_content") val content: ShareGrantContent? = null
)
