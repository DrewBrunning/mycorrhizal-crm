package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.ErrorAggregationResponse
import com.mycorrhizal.crm.model.network.SubsystemHealthResponse
import com.mycorrhizal.crm.model.network.SystemEventsResponse

/**
 * Operational-event timeline data access (issue #424, mirroring web's
 * useSystemEvents over `GET /admin/system-events`). Online-first and
 * deliberately uncached: the timeline is an instance-wide, server-scoped
 * operational record with no offline consumer. Admin-only (the backend 403s
 * the route for non-admins). All filtering is server-side.
 */
interface SystemEventRepository {
    /**
     * GET /admin/system-events — the operational-event timeline, newest
     * first. [limit] is the fetch window (default 100, max 500); the API has
     * no cursor, so "load more" re-fetches with a larger limit. Pass a
     * [correlationId] to retrieve every event in one chain of work.
     */
    suspend fun list(
        component: String? = null,
        severity: String? = null,
        eventType: String? = null,
        correlationId: String? = null,
        ids: List<Long>? = null,
        limit: Int = 100,
    ): Result<SystemEventsResponse>

    /**
     * GET /admin/subsystem-health — the per-subsystem last-known-good state
     * (issue #427), derived on the server from the operational-event stream.
     * No parameters, uncached, admin-only (the backend 403s non-admins).
     */
    suspend fun subsystemHealth(): Result<SubsystemHealthResponse>

    /**
     * GET /admin/error-aggregation — operational failures over [windowHours]
     * bucketed by cause (issue #426), derived on the server from the
     * operational-event stream. Uncached, admin-only.
     */
    suspend fun errorAggregation(windowHours: Int = 24): Result<ErrorAggregationResponse>
}
