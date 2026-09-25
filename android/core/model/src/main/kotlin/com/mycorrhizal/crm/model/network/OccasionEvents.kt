package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * A one-off event the user is hosting (docs/adrs/0026-occasions-events.md,
 * issue #1228) — the Android parity slice of the web Occasions events surface.
 * Unlike the standing [OccasionObligation] rule, an event is a concrete
 * instant (`starts_at`/`ends_at` are RFC 3339) with an invitee/RSVP ledger.
 */
@JsonClass(generateAdapter = true)
data class OccasionEvent(
    val id: String = "",
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "updated_at") val updatedAt: String? = null,
    val title: String = "",
    @Json(name = "starts_at") val startsAt: String = "",
    @Json(name = "ends_at") val endsAt: String? = null,
    val location: String? = null,
    val sensitivity: String = "normal",
    val notes: String? = null,
    /** Change-feed tombstone marker (T17); only present via `?since=`. */
    val deleted: Boolean = false,
)

/** POST/PUT /occasion-events request body. */
@JsonClass(generateAdapter = true)
data class OccasionEventInput(
    val title: String,
    @Json(name = "starts_at") val startsAt: String,
    @Json(name = "ends_at") val endsAt: String? = null,
    val location: String? = null,
    val sensitivity: String = "normal",
    val notes: String? = null,
)

/** GET /occasion-events — cursor-paginated `{ occasion_events, total, next_cursor, limit }`. */
@JsonClass(generateAdapter = true)
data class OccasionEventsResponse(
    @Json(name = "occasion_events") val occasionEvents: List<OccasionEvent> = emptyList(),
    val total: Int = 0,
    @Json(name = "next_cursor") val nextCursor: String? = null,
    val limit: Int = 0,
)

/** POST /occasion-events response — wrapped `{ occasion_event }`. */
@JsonClass(generateAdapter = true)
data class CreateOccasionEventResponse(
    @Json(name = "occasion_event") val occasionEvent: OccasionEvent? = null,
)

/** One invitee/RSVP join row (raw). */
@JsonClass(generateAdapter = true)
data class OccasionEventAttendee(
    val id: String = "",
    @Json(name = "event_id") val eventId: String = "",
    @Json(name = "entity_id") val entityId: String = "",
    val rsvp: String = "pending",
)

/**
 * An attendee enriched with its contact's display data, returned by
 * GET /occasion-events/{id}. `contactId` is the numeric Contact id a client
 * links to; `entityId` is the invited Contact.VCardUID.
 */
@JsonClass(generateAdapter = true)
data class OccasionEventAttendeeView(
    val id: String = "",
    @Json(name = "event_id") val eventId: String = "",
    @Json(name = "entity_id") val entityId: String = "",
    @Json(name = "contact_id") val contactId: Int = 0,
    @Json(name = "contact_name") val contactName: String = "",
    val rsvp: String = "pending",
)

/** GET /occasion-events/{id} — `{ occasion_event, attendees }`. */
@JsonClass(generateAdapter = true)
data class OccasionEventDetail(
    @Json(name = "occasion_event") val occasionEvent: OccasionEvent? = null,
    val attendees: List<OccasionEventAttendeeView> = emptyList(),
)

/** POST /occasion-events/{id}/attendees request body. */
@JsonClass(generateAdapter = true)
data class OccasionEventAttendeeInput(
    @Json(name = "entity_id") val entityId: String,
    val rsvp: String? = null,
)

/** POST /occasion-events/{id}/attendees response — wrapped `{ attendee }`. */
@JsonClass(generateAdapter = true)
data class AddOccasionEventAttendeeResponse(
    val attendee: OccasionEventAttendee? = null,
)

/** PUT /occasion-events/{id}/attendees/{vcard_uid} request body. */
@JsonClass(generateAdapter = true)
data class OccasionEventAttendeeUpdateInput(
    val rsvp: String,
)

/** One candidate from GET /occasion-events/invitee-suggestions. */
@JsonClass(generateAdapter = true)
data class InviteeSuggestion(
    @Json(name = "contact_id") val contactId: Int = 0,
    @Json(name = "contact_name") val contactName: String = "",
    @Json(name = "entity_id") val entityId: String = "",
)

/** GET /occasion-events/invitee-suggestions response. */
@JsonClass(generateAdapter = true)
data class InviteeSuggestionsResponse(
    val suggestions: List<InviteeSuggestion> = emptyList(),
)
