package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.domain.repository.SystemEventRepository
import com.mycorrhizal.crm.model.network.SystemEventsResponse
import com.mycorrhizal.crm.network.ApiClient
import javax.inject.Inject

/**
 * Operational-event timeline access over [ApiClient]. Stateless pass-through —
 * the timeline is server-scoped, instance-wide and immutable, so there is
 * nothing to mirror locally.
 */
class SystemEventRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
) : SystemEventRepository {
    override suspend fun list(
        component: String?,
        severity: String?,
        eventType: String?,
        correlationId: String?,
        limit: Int,
    ): Result<SystemEventsResponse> =
        apiClient.getSystemEvents(
            component = component,
            severity = severity,
            eventType = eventType,
            correlationId = correlationId,
            limit = limit,
        )
}
