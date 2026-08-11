package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.ActivitiesPage
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.ActivityInput

/**
 * Activity (Interaction) data access. Online-first: writes go to the server
 * and the returned record is mirrored into the local cache.
 */
interface ActivityRepository {
    /** A contact's activities. */
    suspend fun listForContact(contactId: Int): Result<List<Activity>>

    /** A single activity (with its participants). */
    suspend fun get(id: Int): Result<Activity>

    /** Create an activity; returns the created record. */
    suspend fun create(input: ActivityInput): Result<Activity>

    /** Update an activity; returns the updated record. */
    suspend fun update(id: Int, input: ActivityInput): Result<Activity>
}
