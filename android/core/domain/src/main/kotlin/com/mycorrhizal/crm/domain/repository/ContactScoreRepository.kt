package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.ContactScoreResponse

/**
 * Relationship health score access (issue #383, ADR-0023). Stateless
 * pass-through — the score is computed server-side, nothing to mirror
 * locally.
 */
interface ContactScoreRepository {
    /** GET /contacts/{id}/score — the full explainable facet breakdown. */
    suspend fun getScore(contactId: Int): Result<ContactScoreResponse>
}
