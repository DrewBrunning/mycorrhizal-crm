package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.domain.repository.SystemEventRepository
import com.mycorrhizal.crm.model.network.ErrorAggregationResponse
import com.mycorrhizal.crm.model.network.JobRunHealthResponse
import com.mycorrhizal.crm.model.network.JobRunsResponse
import com.mycorrhizal.crm.model.network.SubsystemHealthResponse
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
        ids: List<Long>?,
        limit: Int,
    ): Result<SystemEventsResponse> =
        apiClient.getSystemEvents(
            component = component,
            severity = severity,
            eventType = eventType,
            correlationId = correlationId,
            ids = ids,
            limit = limit,
        )

    override suspend fun subsystemHealth(): Result<SubsystemHealthResponse> =
        apiClient.getSubsystemHealth()

    override suspend fun errorAggregation(windowHours: Int): Result<ErrorAggregationResponse> =
        apiClient.getErrorAggregation(windowHours)

    override suspend fun jobRunHealth(): Result<JobRunHealthResponse> =
        apiClient.getJobRunHealth()

    override suspend fun jobRuns(
        jobName: String?,
        result: String?,
        limit: Int,
    ): Result<JobRunsResponse> =
        apiClient.getJobRuns(jobName = jobName, result = result, limit = limit)
}
