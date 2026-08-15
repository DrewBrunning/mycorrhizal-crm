package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.AuditEventsResponse

/**
 * Audit-trail data access (M16, mirroring web's useAudit over T18/T60's
 * backend). Online-first and deliberately uncached: the log is an immutable,
 * server-scoped record with no offline consumer, and undo must always run
 * against the live server state. `entity_type`/`entity_id` filter server-side
 * (the backend does all IDOR gating).
 */
interface AuditRepository {
    /**
     * GET /audit — the caller's immutable event log, newest first.
     * [limit] is the fetch window (default 100, max 500); the API has no
     * cursor, so "load more" re-fetches with a larger limit.
     */
    suspend fun list(
        entityType: String? = null,
        entityId: String? = null,
        limit: Int = 100,
    ): Result<AuditEventsResponse>

    /**
     * POST /audit/:id/undo — reverts a contact-update event. Fails with a
     * 400 for any non-contact / delete event and 410 once the event has aged
     * past AUDIT_RETENTION_DAYS (the purge removes them server-side).
     */
    suspend fun undo(id: Long): Result<Unit>
}
