package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.data.local.CachedRelationshipEdge
import com.mycorrhizal.crm.data.local.CachedRelationshipEdgeDao
import com.mycorrhizal.crm.domain.repository.RelationshipEdgeRepository
import com.mycorrhizal.crm.model.network.RelationshipEdge
import com.mycorrhizal.crm.model.network.RelationshipEdgeInput
import com.mycorrhizal.crm.network.ApiClient
import javax.inject.Inject

/**
 * Online-first relationship-edge access. The list endpoint is full-resync, so
 * fetching a contact's edges replaces the cached rows for that contact. Edges
 * are user-authored content (soft delete server-side); the browse list simply
 * drops deleted edges from the cache.
 */
class RelationshipEdgeRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
    private val dao: CachedRelationshipEdgeDao,
) : RelationshipEdgeRepository {

    override suspend fun listForContact(
        contactId: String,
        status: String?,
        cursor: String?,
    ): Result<List<RelationshipEdge>> {
        val result = apiClient.listRelationshipEdges(
            contactId = contactId,
            status = status,
            cursor = cursor,
        )
        val page = result.getOrNull()
        if (page != null) {
            // Full-resync: wipe the cached edges touching this contact, then
            // write the fresh set (avoids stale rows surviving an unfollow).
            dao.getForContact(contactId).forEach { dao.deleteById(it.id) }
            dao.upsertAll(page.relationshipEdges.filterNot { it.status == "suggested" }.map { it.toCached() })
        }
        return result.map { page -> page.relationshipEdges }
    }

    override suspend fun create(input: RelationshipEdgeInput): Result<RelationshipEdge> {
        val result = apiClient.createRelationshipEdge(input)
        result.getOrNull()?.let { dao.upsert(it.toCached()) }
        return result
    }

    override suspend fun accept(id: String): Result<RelationshipEdge> {
        val result = apiClient.acceptRelationshipEdge(id)
        result.getOrNull()?.let { dao.upsert(it.toCached()) }
        return result
    }

    override suspend fun delete(id: String): Result<Unit> {
        val result = apiClient.deleteRelationshipEdge(id)
        if (result.isSuccess) {
            dao.deleteById(id)
        }
        return result
    }

    private fun RelationshipEdge.toCached(): CachedRelationshipEdge = CachedRelationshipEdge(
        id = id,
        sourceId = sourceId,
        targetId = targetId,
        type = type,
        directional = directional,
        status = status,
        sensitivity = sensitivity,
        updatedAt = updatedAt,
    )
}
