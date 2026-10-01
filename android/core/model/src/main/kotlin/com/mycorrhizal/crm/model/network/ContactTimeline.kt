package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * Issue #1401 (web parity with T78): the contact timeline explorer's wire and
 * domain models for `GET /api/v1/contacts/{id}/timeline` (T66,
 * `TimelineResponse` / `TimelineItem` in `backend/openapi.yaml`).
 */
object TimelineTypes {
    const val NOTE = "note"
    const val ACTIVITY = "activity"
    const val COMPLETION = "completion"
    const val LIFE_EVENT = "life_event"
    const val EXTERNAL_ACTIVITY = "external_activity"
    const val GIFT = "gift"

    /**
     * The six `type` tokens, in the backend's canonical order. Hardcoded mirror of
     * `backend/models/timeline.go`'s `TimelineTypes` (no dynamic type-list endpoint
     * exists, by design) — keep in sync by hand.
     */
    val ALL: List<String> = listOf(NOTE, ACTIVITY, COMPLETION, LIFE_EVENT, EXTERNAL_ACTIVITY, GIFT)
}

/**
 * The `bucket` recency filter tokens. Hardcoded mirror of the backend's
 * `TimelineBucket*` constants (`all` is the default and is never sent).
 */
object TimelineBuckets {
    const val ALL = "all"
    const val LAST_7_DAYS = "last_7_days"
    const val LAST_30_DAYS = "last_30_days"
    const val LAST_90_DAYS = "last_90_days"
    const val THIS_YEAR = "this_year"
    val ALL_BUCKETS: List<String> = listOf(ALL, LAST_7_DAYS, LAST_30_DAYS, LAST_90_DAYS, THIS_YEAR)
}

/** Wire shape of one `TimelineItem`: the raw entity sits under `data`, whose shape follows `type`. */
@JsonClass(generateAdapter = true)
data class TimelineWireItem(
    val type: String = "",
    val id: String = "",
    val date: String = "",
    val data: Map<String, Any?>? = null,
)

/** Wire shape of `TimelineResponse`. `items` is `[]` when empty; `next_cursor` is empty on the last page. */
@JsonClass(generateAdapter = true)
data class TimelineWirePage(
    val items: List<TimelineWireItem>? = null,
    @Json(name = "next_cursor") val nextCursor: String? = null,
    val limit: Int = 0,
)

/**
 * One decoded timeline event: exactly one of the typed entity fields is set,
 * matching [type]. Kept as a flat holder so `core:model` stays free of the
 * feature module's UI-facing `TimelineItem`.
 */
data class TimelineEvent(
    val type: String,
    val id: String,
    val date: String,
    val note: Note? = null,
    val activity: Activity? = null,
    val completion: ReminderCompletion? = null,
    val lifeEvent: LifeEvent? = null,
    val externalActivity: ExternalActivity? = null,
    val gift: Gift? = null,
)

/** A decoded page of the contact timeline. A null/empty [nextCursor] means no more rows. */
data class ContactTimelinePage(
    val items: List<TimelineEvent>,
    val nextCursor: String?,
    val limit: Int,
)
