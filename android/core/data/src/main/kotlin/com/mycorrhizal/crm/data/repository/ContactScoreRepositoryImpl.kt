package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.domain.repository.ContactScoreRepository
import com.mycorrhizal.crm.model.network.ContactScoreResponse
import com.mycorrhizal.crm.network.ApiClient
import javax.inject.Inject

/**
 * Relationship health score access over [ApiClient] (issue #383). Stateless
 * pass-through — the score is computed server-side, so there is nothing to
 * mirror locally.
 */
class ContactScoreRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
) : ContactScoreRepository {

    override suspend fun getScore(contactId: Int): Result<ContactScoreResponse> =
        apiClient.getContactScore(contactId)
}
