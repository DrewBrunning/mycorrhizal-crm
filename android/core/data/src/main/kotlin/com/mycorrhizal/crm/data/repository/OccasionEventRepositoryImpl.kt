package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.data.local.CachedOccasionEvent
import com.mycorrhizal.crm.data.local.CachedOccasionEventDao
import com.mycorrhizal.crm.domain.repository.OccasionEventRepository
import com.mycorrhizal.crm.model.network.InviteeSuggestion
import com.mycorrhizal.crm.model.network.OccasionEvent
import com.mycorrhizal.crm.model.network.OccasionEventAttendee
import com.mycorrhizal.crm.model.network.OccasionEventAttendeeInput
import com.mycorrhizal.crm.model.network.OccasionEventDetail
import com.mycorrhizal.crm.model.network.OccasionEventInput
import com.mycorrhizal.crm.network.ApiClient
import javax.inject.Inject

/**
 * Online-first occasion-event access (docs/adrs/0026-occasions-events.md, issue
 * #1228), following the cadence/timeline full-resync pattern: listing replaces
 * the cached rows and writes mirror the returned record. Attendee operations
 * pass straight through to the server — the detail carries the live roster.
 */
class OccasionEventRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
    private val dao: CachedOccasionEventDao,
) : OccasionEventRepository {
    override suspend fun list(): Result<List<OccasionEvent>> {
        val result = apiClient.listOccasionEvents()
        val page = result.getOrNull()
        if (page != null) {
            dao.deleteAll()
            dao.upsertAll(page.occasionEvents.filterNot { it.deleted }.map { it.toCached() })
        }
        return result.map { it.occasionEvents }
    }

    override suspend fun get(id: String): Result<OccasionEventDetail> = apiClient.getOccasionEvent(id)

    override suspend fun create(input: OccasionEventInput): Result<OccasionEvent> {
        val result = apiClient.createOccasionEvent(input)
        result.getOrNull()?.let { dao.upsert(it.toCached()) }
        return result
    }

    override suspend fun update(id: String, input: OccasionEventInput): Result<OccasionEvent> {
        val result = apiClient.updateOccasionEvent(id, input)
        result.getOrNull()?.let { dao.upsert(it.toCached()) }
        return result
    }

    override suspend fun delete(id: String): Result<Unit> {
        val result = apiClient.deleteOccasionEvent(id)
        if (result.isSuccess) dao.deleteById(id)
        return result
    }

    override suspend fun addAttendee(
        eventId: String,
        input: OccasionEventAttendeeInput,
    ): Result<OccasionEventAttendee> = apiClient.addOccasionEventAttendee(eventId, input)

    override suspend fun updateAttendee(
        eventId: String,
        vcardUid: String,
        rsvp: String,
    ): Result<OccasionEventAttendee> = apiClient.updateOccasionEventAttendee(eventId, vcardUid, rsvp)

    override suspend fun removeAttendee(eventId: String, vcardUid: String): Result<Unit> =
        apiClient.removeOccasionEventAttendee(eventId, vcardUid)

    override suspend fun suggestInvitees(
        circleIds: List<String>,
        eventId: String?,
    ): Result<List<InviteeSuggestion>> =
        apiClient.suggestInvitees(circleIds, eventId).map { it.suggestions }

    private fun OccasionEvent.toCached(): CachedOccasionEvent = CachedOccasionEvent(
        id = id,
        title = title,
        startsAt = startsAt,
        endsAt = endsAt,
        location = location,
        sensitivity = sensitivity,
        notes = notes,
        updatedAt = updatedAt,
        deleted = deleted,
    )
}
