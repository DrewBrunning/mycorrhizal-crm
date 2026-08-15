package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.domain.repository.CircleWithMembers
import com.mycorrhizal.crm.domain.repository.GraphRepository
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.CircleMember
import com.mycorrhizal.crm.model.network.CirclesPage
import com.mycorrhizal.crm.model.network.GraphConnectionsResponse
import com.mycorrhizal.crm.network.ApiClient
import javax.inject.Inject

/**
 * Ego-centric graph access over [ApiClient]. Stateless pass-through — the
 * traversal is computed server-side, so there is nothing to mirror locally.
 */
class GraphRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
) : GraphRepository {

    override suspend fun getConnections(
        from: String,
        depth: Int?,
        relation: String?,
    ): Result<GraphConnectionsResponse> =
        apiClient.getConnections(from = from, depth = depth, relation = relation)

    override suspend fun selfContactVCardUid(): Result<String?> =
        apiClient.currentUser().map { it.selfContactVCardUid }

    override suspend fun circlesWithMembers(): Result<List<CircleWithMembers>> {
        // One page rather than walking the cursor: a circle filter is a
        // quick-render concern, and 100 is the backend's maxLimit — a user
        // with more circles than that is a documented truncation (matches
        // CircleRepositoryImpl.circlesForContact's own choice).
        val page = apiClient.listCircles(includeMembers = true, limit = 100).getOrElse {
            return Result.failure(it)
        }
        return Result.success(page.toCirclesWithMembers())
    }
}

/** [CirclesPage] -> circles with their member VCardUIDs (pure, unit-testable). */
fun CirclesPage.toCirclesWithMembers(): List<CircleWithMembers> {
    val byCircle = members.orEmpty().groupBy(CircleMember::circleId)
    return circles.map { circle: Circle ->
        CircleWithMembers(
            id = circle.id,
            name = circle.name,
            memberVCardUids = byCircle[circle.id].orEmpty().map(CircleMember::memberVCardUid).toSet(),
        )
    }
}
