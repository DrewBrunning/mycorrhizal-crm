package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.AcceptHouseholdSuggestionInput
import com.mycorrhizal.crm.model.network.AddressHouseholdSuggestion
import com.mycorrhizal.crm.model.network.DismissHouseholdSuggestionInput
import com.mycorrhizal.crm.model.network.Household
import com.mycorrhizal.crm.model.network.HouseholdMember
import com.mycorrhizal.crm.model.network.RelationshipEdge

/**
 * Household data access. Online-first: writes go to the server and the
 * returned records are mirrored into the local cache. Households use UUID
 * string PKs and carry a type (family_unit|roommates|other).
 */
interface HouseholdRepository {
    /** All households, cursor-paginated. */
    suspend fun list(cursor: String? = null, limit: Int = 100): Result<List<Household>>

    /** A household with its members. */
    suspend fun getWithMembers(id: String): Result<HouseholdDetail>

    /** Create a household; returns the created household. */
    suspend fun create(name: String, type: String): Result<Household>

    /** Update a household (name/type); returns the updated household. */
    suspend fun update(id: String, name: String, type: String): Result<Household>

    /** Delete a household (hard delete). */
    suspend fun delete(id: String): Result<Unit>

    /** Add a member (by contact VCard UID) to a household. */
    suspend fun addMember(id: String, vcardUid: String, role: String? = null): Result<HouseholdMember>

    /** Remove a member from a household (hard delete join row). */
    suspend fun removeMember(id: String, vcardUid: String): Result<Unit>

    /** Update a member's role/since/until. */
    suspend fun updateMember(id: String, vcardUid: String, role: String? = null): Result<Unit>

    /** Trigger the relationship-suggestion engine for a household; returns the newly created suggested edges. */
    suspend fun suggestRelationships(id: String): Result<List<RelationshipEdge>>

    /** T40: scan the user's contacts for shared-address household suggestions (read-only). */
    suspend fun suggestAddressHouseholds(): Result<List<AddressHouseholdSuggestion>>

    /** T40: materialize a household from a suggested group; returns the created household. */
    suspend fun acceptAddressSuggestion(input: AcceptHouseholdSuggestionInput): Result<Household>

    /** T40: permanently dismiss a suggested group so the scan stops offering it. */
    suspend fun dismissAddressSuggestion(input: DismissHouseholdSuggestionInput): Result<Unit>
}

data class HouseholdDetail(
    val household: Household,
    val members: List<HouseholdMember>,
)
