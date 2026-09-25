package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.InviteeSuggestion
import com.mycorrhizal.crm.model.network.OccasionEvent
import com.mycorrhizal.crm.model.network.OccasionEventAttendee
import com.mycorrhizal.crm.model.network.OccasionEventAttendeeInput
import com.mycorrhizal.crm.model.network.OccasionEventDetail
import com.mycorrhizal.crm.model.network.OccasionEventInput

/**
 * Occasion-event data access (docs/adrs/0026-occasions-events.md, issue #1228).
 * Online-first: writes go to the server and the returned record is mirrored
 * into the local cache. An event is user-scoped (no `entity_id`); attendees are
 * a nested sub-resource fetched with the event detail.
 */
interface OccasionEventRepository {
    /** The signed-in user's events, newest-updated first. */
    suspend fun list(): Result<List<OccasionEvent>>

    /** A single event plus its attendee/RSVP list. */
    suspend fun get(id: String): Result<OccasionEventDetail>

    /** Create an event; returns the created event. */
    suspend fun create(input: OccasionEventInput): Result<OccasionEvent>

    /** Update an event (full replace); returns the updated event. */
    suspend fun update(id: String, input: OccasionEventInput): Result<OccasionEvent>

    /** Delete an event (soft delete server-side; attendees hard-delete with it). */
    suspend fun delete(id: String): Result<Unit>

    /** Invite one owned contact to an event. */
    suspend fun addAttendee(eventId: String, input: OccasionEventAttendeeInput): Result<OccasionEventAttendee>

    /** Record an attendee's RSVP status. */
    suspend fun updateAttendee(eventId: String, vcardUid: String, rsvp: String): Result<OccasionEventAttendee>

    /** Remove one contact from an event's attendee list. */
    suspend fun removeAttendee(eventId: String, vcardUid: String): Result<Unit>

    /** Candidate invitees from one or more circles, excluding those already invited. */
    suspend fun suggestInvitees(circleIds: List<String>, eventId: String?): Result<List<InviteeSuggestion>>
}
