package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.data.local.CachedActivity
import com.mycorrhizal.crm.data.local.CachedActivityDao
import com.mycorrhizal.crm.domain.repository.ActivityRepository
import com.mycorrhizal.crm.domain.repository.ContactActivitiesPage
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.ActivitiesPage
import com.mycorrhizal.crm.model.network.ActivityInput
import com.mycorrhizal.crm.network.ApiClient
import javax.inject.Inject

/**
 * Online-first activity access. Writes go to the server; successful responses
 * are mirrored into the Room cache. Reads are not cached-offline in Phase 2
 * (the activities list is always fetched), but the mirror keeps the table warm
 * for a later timeline phase.
 */
class ActivityRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
    private val dao: CachedActivityDao,
) : ActivityRepository {

    override suspend fun listForContact(
        contactId: Int,
        cursor: String?,
        limit: Int?,
        search: String?,
        fromDate: String?,
        toDate: String?,
    ): Result<ContactActivitiesPage> {
        val result = apiClient.listContactActivities(contactId, cursor, limit, search, fromDate, toDate)
        val response = result.getOrNull()
        if (response != null) {
            dao.upsertAll(response.activities.map { it.toCached() })
        }
        return result.map { response -> ContactActivitiesPage(activities = response.activities, nextCursor = response.nextCursor) }
    }

    override suspend fun listAll(cursor: String?, limit: Int?): Result<ActivitiesPage> {
        val result = apiClient.listActivities(cursor = cursor, limit = limit, includeContacts = true)
        result.getOrNull()?.let { page -> dao.upsertAll(page.activities.map { it.toCached() }) }
        return result
    }

    override suspend fun get(id: Int): Result<Activity> = apiClient.getActivity(id)

    override suspend fun create(input: ActivityInput): Result<Activity> {
        val result = apiClient.createActivity(input)
        result.getOrNull()?.let { dao.upsert(it.toCached()) }
        return result
    }

    override suspend fun update(id: Int, input: ActivityInput): Result<Activity> {
        val result = apiClient.updateActivity(id, input)
        result.getOrNull()?.let { dao.upsert(it.toCached()) }
        return result
    }

    override suspend fun delete(id: Int): Result<Unit> {
        val result = apiClient.deleteActivity(id)
        if (result.isSuccess) dao.deleteById(id)
        return result
    }

    private fun Activity.toCached(): CachedActivity = CachedActivity(
        id = id,
        title = title,
        description = description,
        location = location,
        date = date,
        type = type,
        deleted = deleted,
    )
}
