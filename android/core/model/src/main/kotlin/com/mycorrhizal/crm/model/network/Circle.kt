package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

@JsonClass(generateAdapter = true)
data class Circle(
    val id: String = "",
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "updated_at") val updatedAt: String? = null,
    val name: String = "",
    /** Issue #1995: free text, up to [CircleColors.MAX_DESCRIPTION] chars; "" when unset. */
    val description: String = "",
    /** Issue #1995: a [CircleColors.ALL] token, or "" for no colour. */
    val color: String = ""
)

/**
 * Issue #1995 palette tokens. Hardcoded mirror of the backend `models.CircleColors` (and the web
 * `CIRCLE_COLORS`) — there is no dynamic list endpoint (Android AGENTS.md "Client mirrors";
 * frontend trap #4). Keep in sync by hand; the backend rejects anything else with
 * `circle_color_invalid`. `""` means "no colour" and is not in [ALL].
 */
object CircleColors {
    val ALL: List<String> =
        listOf("red", "orange", "yellow", "green", "teal", "blue", "purple", "pink", "grey")

    /** Mirror of the web `CIRCLE_DESCRIPTION_MAX` / the backend description cap. */
    const val MAX_DESCRIPTION = 500
}

@JsonClass(generateAdapter = true)
data class CircleMember(
    val id: Int = 0,
    @Json(name = "circle_id") val circleId: String = "",
    @Json(name = "member_vcard_uid") val memberVCardUid: String = ""
)

/** GET /circles response — cursor-paginated, members only when include_members=true. */
@JsonClass(generateAdapter = true)
data class CirclesPage(
    val circles: List<Circle> = emptyList(),
    val total: Int = 0,
    @Json(name = "next_cursor") val nextCursor: String? = null,
    val limit: Int = 0,
    val members: List<CircleMember>? = null,
    val sync: SyncInfo? = null
)

/** POST /circles response — wrapped `{ message, circle }` (ticket §2.6). */
@JsonClass(generateAdapter = true)
data class CreateCircleResponse(val message: String? = null, val circle: Circle? = null)

/** GET /circles/{id} response — `{ circle, members }`. */
@JsonClass(generateAdapter = true)
data class CircleDetailResponse(val circle: Circle? = null, val members: List<CircleMember>? = null)

/** POST /circles/{id}/members response — `{ message, member }`. */
@JsonClass(generateAdapter = true)
data class AddCircleMemberResponse(val message: String? = null, val member: CircleMember? = null)

/**
 * POST/PUT /circles body. Membership is a sub-resource. [description] and [color] follow the
 * backend's pointer semantics: null (key omitted) keeps the stored value on update, "" clears it.
 */
@JsonClass(generateAdapter = true)
data class CircleInput(val name: String, val description: String? = null, val color: String? = null)

/** The live circle member a circle-timeline entry is about (issue #2361). */
@JsonClass(generateAdapter = true)
data class CircleTimelineMember(
    val id: Int = 0,
    val firstname: String = "",
    val lastname: String = ""
) {
    fun displayName(): String = "$firstname $lastname".trim()
}

/**
 * One entry of `GET /circles/{id}/timeline`: the contact-timeline item shape plus the member(s)
 * it is about. [data] is the raw entity (note/activity/…); the circle UI only needs a one-line
 * summary, so it is not decoded into typed entities.
 */
@JsonClass(generateAdapter = true)
data class CircleTimelineItem(
    val type: String = "",
    val id: String = "",
    val date: String = "",
    val data: Map<String, Any?>? = null,
    val members: List<CircleTimelineMember> = emptyList()
)

/** GET /circles/{id}/timeline — an empty `next_cursor` (no more rows) is mapped to null. */
@JsonClass(generateAdapter = true)
data class CircleTimelinePage(
    val items: List<CircleTimelineItem> = emptyList(),
    @Json(name = "next_cursor") val nextCursor: String? = null,
    val limit: Int = 0,
    /** Members in the circle (archived members count but contribute no events). */
    @Json(name = "member_count") val memberCount: Int = 0
)

/** POST /circles/{id}/members body. */
@JsonClass(generateAdapter = true)
data class CircleMemberInput(@Json(name = "member_vcard_uid") val memberVCardUid: String)
