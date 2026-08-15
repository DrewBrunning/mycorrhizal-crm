package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.data.local.CachedHousehold
import com.mycorrhizal.crm.data.local.CachedHouseholdDao
import com.mycorrhizal.crm.data.local.CachedHouseholdMember
import com.mycorrhizal.crm.data.local.CachedHouseholdMemberDao
import com.mycorrhizal.crm.domain.repository.HouseholdDetail
import com.mycorrhizal.crm.domain.repository.HouseholdRepository
import com.mycorrhizal.crm.model.network.AcceptHouseholdSuggestionInput
import com.mycorrhizal.crm.model.network.AddressHouseholdSuggestion
import com.mycorrhizal.crm.model.network.DismissHouseholdSuggestionInput
import com.mycorrhizal.crm.model.network.Household
import com.mycorrhizal.crm.model.network.HouseholdInput
import com.mycorrhizal.crm.model.network.HouseholdMember
import com.mycorrhizal.crm.model.network.HouseholdMemberInput
import com.mycorrhizal.crm.model.network.RelationshipEdge
import com.mycorrhizal.crm.network.ApiClient
import javax.inject.Inject

/**
 * Online-first household access. Writes go to the server; successful responses
 * are mirrored into the Room cache. Membership join rows hard-delete.
 */
class HouseholdRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
    private val householdDao: CachedHouseholdDao,
    private val memberDao: CachedHouseholdMemberDao,
) : HouseholdRepository {

    override suspend fun list(cursor: String?, limit: Int): Result<List<Household>> {
        val result = apiClient.listHouseholds(cursor = cursor, limit = limit)
        val page = result.getOrNull()
        if (page != null) {
            householdDao.upsertAll(page.households.map { it.toCached() })
        }
        return result.map { page -> page.households }
    }

    override suspend fun getWithMembers(id: String): Result<HouseholdDetail> {
        val result = apiClient.getHousehold(id)
        val detail = result.getOrNull()
        val household = detail?.household
        if (household != null) {
            householdDao.upsert(household.toCached())
            val members = detail.members.orEmpty()
            memberDao.deleteByHouseholdId(id)
            memberDao.upsertAll(members.map { it.toCached() })
            return Result.success(HouseholdDetail(household = household, members = members))
        }
        return result.map { d ->
            HouseholdDetail(household = d.household ?: Household(), members = d.members.orEmpty())
        }
    }

    override suspend fun create(name: String, type: String): Result<Household> {
        val result = apiClient.createHousehold(HouseholdInput(name = name, type = type))
        result.getOrNull()?.let { householdDao.upsert(it.toCached()) }
        return result
    }

    override suspend fun update(id: String, name: String, type: String): Result<Household> {
        val result = apiClient.updateHousehold(id, HouseholdInput(name = name, type = type))
        result.getOrNull()?.let { householdDao.upsert(it.toCached()) }
        return result
    }

    override suspend fun delete(id: String): Result<Unit> {
        val result = apiClient.deleteHousehold(id)
        if (result.isSuccess) {
            householdDao.deleteById(id)
            memberDao.deleteByHouseholdId(id)
        }
        return result
    }

    override suspend fun addMember(id: String, vcardUid: String, role: String?): Result<HouseholdMember> {
        val result = apiClient.addHouseholdMember(
            id,
            // role must never serialize as null: the backend's member DTOs bind
            // `role` into a Go `string`, which rejects an explicit JSON null
            // with a 400. "" means "no role" (and the backend's `omitempty`
            // drops it entirely on add).
            HouseholdMemberInput(memberVCardUid = vcardUid, role = role.orEmpty()),
        )
        result.getOrNull()?.let { memberDao.upsertAll(listOf(it.toCached())) }
        return result
    }

    override suspend fun removeMember(id: String, vcardUid: String): Result<Unit> {
        val result = apiClient.removeHouseholdMember(id, vcardUid)
        if (result.isSuccess) {
            memberDao.deleteMember(id, vcardUid)
        }
        return result
    }

    override suspend fun updateMember(id: String, vcardUid: String, role: String?): Result<Unit> {
        val result = apiClient.updateHouseholdMember(
            id,
            vcardUid,
            // Same null-role rule as addMember: the backend's PATCH binds role
            // into a plain Go string, so an explicit JSON null is a 400. ""
            // clears the role (matches web's `role: ''`).
            HouseholdMemberInput(memberVCardUid = vcardUid, role = role.orEmpty()),
        )
        if (result.isSuccess) {
            // Re-pull the member rows so the cache reflects the new role.
            memberDao.deleteByHouseholdId(id)
            apiClient.getHousehold(id).getOrNull()?.members?.let { members ->
                memberDao.upsertAll(members.map { it.toCached() })
            }
        }
        return result
    }

    override suspend fun suggestRelationships(id: String): Result<List<RelationshipEdge>> =
        apiClient.suggestHouseholdRelationships(id).map { it.suggestedEdges }

    override suspend fun suggestAddressHouseholds(): Result<List<AddressHouseholdSuggestion>> =
        apiClient.suggestAddressHouseholds().map { it.suggestions }

    override suspend fun acceptAddressSuggestion(input: AcceptHouseholdSuggestionInput): Result<Household> =
        apiClient.acceptHouseholdSuggestion(input).map { household ->
            householdDao.upsert(household.toCached())
            household
        }

    override suspend fun dismissAddressSuggestion(input: DismissHouseholdSuggestionInput): Result<Unit> =
        apiClient.dismissHouseholdSuggestion(input)

    private fun Household.toCached(): CachedHousehold = CachedHousehold(
        id = id,
        name = name,
        type = type,
        updatedAt = updatedAt,
    )

    private fun HouseholdMember.toCached(): CachedHouseholdMember = CachedHouseholdMember(
        id = id,
        householdId = householdId,
        memberVCardUid = memberVCardUid,
        role = role,
        since = since,
        until = until,
    )
}
