package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.domain.repository.ContactTimelineRepository
import com.mycorrhizal.crm.model.network.ContactTimelinePage
import com.mycorrhizal.crm.network.ApiClient
import javax.inject.Inject

/** Delegates straight to the [ApiClient]; see [ContactTimelineRepository] for why nothing is cached. */
class ContactTimelineRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
) : ContactTimelineRepository {

    override suspend fun page(
        contactId: Int,
        types: Collection<String>,
        bucket: String?,
        cursor: String?,
        limit: Int,
    ): Result<ContactTimelinePage> = apiClient.getContactTimeline(contactId, types, bucket, cursor, limit)
}
