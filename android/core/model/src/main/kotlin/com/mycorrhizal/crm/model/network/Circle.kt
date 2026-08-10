package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

@JsonClass(generateAdapter = true)
data class Circle(
    val id: String = "",
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "updated_at") val updatedAt: String? = null,
    val name: String = "",
)

@JsonClass(generateAdapter = true)
data class CircleMember(
    val id: Int = 0,
    @Json(name = "circle_id") val circleId: String = "",
    @Json(name = "member_vcard_uid") val memberVCardUid: String = "",
)

/** GET /circles response — cursor-paginated, members only when include_members=true. */
@JsonClass(generateAdapter = true)
data class CirclesPage(
    val circles: List<Circle> = emptyList(),
    val total: Int = 0,
    @Json(name = "next_cursor") val nextCursor: String? = null,
    val limit: Int = 0,
    val members: List<CircleMember>? = null,
    val sync: SyncInfo? = null,
)

/** POST /circles response — wrapped `{ message, circle }` (ticket §2.6). */
@JsonClass(generateAdapter = true)
data class CreateCircleResponse(
    val message: String? = null,
    val circle: Circle? = null,
)

/** GET /circles/{id} response — `{ circle, members }`. */
@JsonClass(generateAdapter = true)
data class CircleDetailResponse(
    val circle: Circle? = null,
    val members: List<CircleMember>? = null,
)

/** POST /circles/{id}/members response — `{ message, member }`. */
@JsonClass(generateAdapter = true)
data class AddCircleMemberResponse(
    val message: String? = null,
    val member: CircleMember? = null,
)

/** POST /circles body — the API only models name (membership is a sub-resource). */
@JsonClass(generateAdapter = true)
data class CircleInput(
    val name: String,
)

/** POST /circles/{id}/members body. */
@JsonClass(generateAdapter = true)
data class CircleMemberInput(
    @Json(name = "member_vcard_uid") val memberVCardUid: String,
)
