package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.domain.repository.AuditRepository
import com.mycorrhizal.crm.model.network.AuditEventsResponse
import com.mycorrhizal.crm.network.ApiClient
import javax.inject.Inject

/**
 * Audit-trail access over [ApiClient]. Stateless pass-through — the audit log
 * is server-scoped and immutable, so there is nothing to mirror locally.
 */
class AuditRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
) : AuditRepository {
    override suspend fun list(
        entityType: String?,
        entityId: String?,
        limit: Int,
    ): Result<AuditEventsResponse> =
        apiClient.getAuditEvents(entityType = entityType, entityId = entityId, limit = limit)

    override suspend fun undo(id: Long): Result<Unit> =
        apiClient.undoAuditEvent(id).map { Unit }
}
