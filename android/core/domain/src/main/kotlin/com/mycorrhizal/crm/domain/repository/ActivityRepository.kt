package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.ActivitiesPage
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.ActivityInput

/** A page of one contact's activities (M19), with its T17 cursor-pagination state. */
data class ContactActivitiesPage(
    val activities: List<Activity>,
    val nextCursor: String?,
)

/**
 * Activity (Interaction) data access. Online-first: writes go to the server
 * and the returned record is mirrored into the local cache.
 */
interface ActivityRepository {
    /**
     * A page of a contact's activities (M19: search/date-filtered, cursor-paginated).
     * [search] is free-text on title/description/location; [fromDate]/[toDate]
     * are `YYYY-MM-DD` bounds on the activity date, both inclusive. Each row
     * carries its participant contacts.
     */
    suspend fun listForContact(
        contactId: Int,
        cursor: String? = null,
        limit: Int? = null,
        search: String? = null,
        fromDate: String? = null,
        toDate: String? = null,
    ): Result<ContactActivitiesPage>

    /**
     * All activities across every contact (M9 Activities drawer entry), with participants
     * attached — `GET /activities?include=contacts`, cursor-paginated.
     */
    suspend fun listAll(cursor: String? = null, limit: Int? = null): Result<ActivitiesPage>

    /** A single activity (with its participants). */
    suspend fun get(id: Int): Result<Activity>

    /** Create an activity; returns the created record. */
    suspend fun create(input: ActivityInput): Result<Activity>

    /** Update an activity; returns the updated record. */
    suspend fun update(id: Int, input: ActivityInput): Result<Activity>

    /** Delete an activity (soft delete server-side; removes the local cache row). */
    suspend fun delete(id: Int): Result<Unit>
}
