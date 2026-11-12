package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * A standing, recurring obligation toward a contact — "this contact is on my holiday card
 * list every year" (docs/adrs/0024-occasions.md, issue #2521). Mirrors the backend
 * `models.OccasionObligation` and web's `OccasionObligation` (`frontend/src/api/occasionObligations.ts`).
 * [entityId] is the subject contact's `VCardUID`. [kind] is an open classifier (see
 * [OccasionObligationKinds]); both anchors are set or both are absent.
 */
@JsonClass(generateAdapter = true)
data class OccasionObligation(
    val id: String = "",
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "updated_at") val updatedAt: String? = null,
    @Json(name = "entity_id") val entityId: String = "",
    val kind: String = "",
    val label: String = "",
    @Json(name = "anchor_month") val anchorMonth: Int? = null,
    @Json(name = "anchor_day") val anchorDay: Int? = null,
    @Json(name = "linked_life_event_id") val linkedLifeEventId: String? = null,
    @Json(name = "lead_time_days") val leadTimeDays: Int = 0,
    val active: Boolean = true,
    val sensitivity: String = "normal",
    val notes: String? = null,
    /** Change-feed tombstone marker (T17); only present via `?since=`. */
    val deleted: Boolean = false
) {
    /** Only an obligation with a month AND a day surfaces on the upcoming-occasions list. */
    val hasAnchor: Boolean get() = anchorMonth != null && anchorDay != null
}

/**
 * POST/PUT /occasion-obligations request body (`models.OccasionObligationInput`). PUT is a full
 * replace, so an unset [anchorMonth]/[anchorDay] clears the anchor and [linkedLifeEventId] must be
 * carried forward by the caller.
 */
@JsonClass(generateAdapter = true)
data class OccasionObligationInput(
    @Json(name = "entity_id") val entityId: String,
    val kind: String,
    val label: String,
    @Json(name = "anchor_month") val anchorMonth: Int? = null,
    @Json(name = "anchor_day") val anchorDay: Int? = null,
    @Json(name = "linked_life_event_id") val linkedLifeEventId: String? = null,
    @Json(name = "lead_time_days") val leadTimeDays: Int = 0,
    val active: Boolean = true,
    val sensitivity: String = "normal",
    val notes: String? = null
)

/** GET /occasion-obligations — cursor-paginated `{ occasion_obligations, total, next_cursor, limit }`. */
@JsonClass(generateAdapter = true)
data class OccasionObligationsResponse(
    @Json(name = "occasion_obligations") val occasionObligations: List<OccasionObligation> =
        emptyList(),
    val total: Int = 0,
    @Json(name = "next_cursor") val nextCursor: String? = null,
    val limit: Int = 0
)

/** POST /occasion-obligations response — wrapped `{ message, occasion_obligation }`. */
@JsonClass(generateAdapter = true)
data class CreateOccasionObligationResponse(
    @Json(name = "occasion_obligation") val occasionObligation: OccasionObligation? = null
)

/**
 * The three kinds the backend names (`models.OccasionObligationKind*`). The server column is
 * free text, so these are only suggestions (a hardcoded mirror, frontend trap #4).
 */
object OccasionObligationKinds {
    const val CARD = "card"
    const val GIFT = "gift"
    const val INVITE = "invite"

    val ALL = listOf(CARD, GIFT, INVITE)
}
