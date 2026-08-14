package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.data.local.CachedCadencePolicy
import com.mycorrhizal.crm.data.local.CachedCadencePolicyDao
import com.mycorrhizal.crm.domain.repository.CadencePolicyRepository
import com.mycorrhizal.crm.model.network.CadencePolicy
import com.mycorrhizal.crm.model.network.CadencePolicyInput
import com.mycorrhizal.crm.network.ApiClient
import javax.inject.Inject

/**
 * Online-first cadence-policy access, following the timeline-entity full-resync
 * pattern: listing a contact's policies replaces the cached rows; writes mirror
 * the returned record. Cadence policies are user-authored content (soft delete
 * server-side), so tombstones are filtered the way the timeline entities do.
 */
class CadencePolicyRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
    private val dao: CachedCadencePolicyDao,
) : CadencePolicyRepository {
    override suspend fun listForContact(entityId: String): Result<List<CadencePolicy>> {
        val result = apiClient.listCadencePolicies(entityId = entityId)
        val page = result.getOrNull()
        if (page != null) {
            dao.deleteForContact(entityId)
            dao.upsertAll(page.cadencePolicies.filterNot { it.deleted }.map { it.toCached(entityId) })
        }
        return result.map { it.cadencePolicies }
    }

    override suspend fun get(id: String): Result<CadencePolicy> = apiClient.getCadencePolicy(id)

    override suspend fun create(input: CadencePolicyInput): Result<CadencePolicy> {
        val result = apiClient.createCadencePolicy(input)
        result.getOrNull()?.let { dao.upsert(it.toCached(input.entityId)) }
        return result
    }

    override suspend fun update(id: String, input: CadencePolicyInput): Result<CadencePolicy> {
        val result = apiClient.updateCadencePolicy(id, input)
        result.getOrNull()?.let { dao.upsert(it.toCached(input.entityId)) }
        return result
    }

    override suspend fun delete(id: String): Result<Unit> {
        val result = apiClient.deleteCadencePolicy(id)
        if (result.isSuccess) dao.deleteById(id)
        return result
    }

    private fun CadencePolicy.toCached(entityId: String): CachedCadencePolicy = CachedCadencePolicy(
        id = id,
        entityId = entityId,
        targetIntervalDays = targetIntervalDays,
        qualifyingTypes = qualifyingTypes,
        updatedAt = updatedAt,
        deleted = deleted,
    )
}
