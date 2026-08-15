package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * Mirrors backend/models/household.go's HouseholdType* constants and the
 * `oneof=family_unit roommates other` validator (frontend trap 4 — hardcoded
 * mirror, kept in sync by hand).
 */
object HouseholdTypes {
    const val FAMILY_UNIT = "family_unit"
    const val ROOMMATES = "roommates"
    const val OTHER = "other"
    val ALL: List<String> = listOf(FAMILY_UNIT, ROOMMATES, OTHER)
}

/**
 * Conventional (not enforced) role tokens, mirroring backend/models/
 * household.go's HouseholdRole* constants (frontend trap 4 — hardcoded
 * mirror, kept in sync by hand).
 */
object HouseholdRoles {
    const val ADULT = "adult"
    const val CHILD = "child"
    const val PET = "pet"
    const val ROOMMATE = "roommate"
    val ALL: List<String> = listOf(ADULT, CHILD, PET, ROOMMATE)
}

@JsonClass(generateAdapter = true)
data class Household(
    val id: String = "",
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "updated_at") val updatedAt: String? = null,
    val name: String = "",
    val type: String = HouseholdTypes.FAMILY_UNIT,
    val address: Address? = null,
)

@JsonClass(generateAdapter = true)
data class HouseholdMember(
    val id: Int = 0,
    @Json(name = "household_id") val householdId: String = "",
    @Json(name = "member_vcard_uid") val memberVCardUid: String = "",
    val role: String? = null,
    val since: String? = null,
    val until: String? = null,
)

/** GET /households response — cursor-paginated, members when include_members=true. */
@JsonClass(generateAdapter = true)
data class HouseholdsPage(
    val households: List<Household> = emptyList(),
    val total: Int = 0,
    @Json(name = "next_cursor") val nextCursor: String? = null,
    val limit: Int = 0,
    val members: List<HouseholdMember>? = null,
    val sync: SyncInfo? = null,
)

/** POST /households response — wrapped `{ message, household }` (ticket §2.6). */
@JsonClass(generateAdapter = true)
data class CreateHouseholdResponse(
    val message: String? = null,
    val household: Household? = null,
)

/** GET /households/{id} response — `{ household, members }`. */
@JsonClass(generateAdapter = true)
data class HouseholdDetailResponse(
    val household: Household? = null,
    val members: List<HouseholdMember>? = null,
)

/** POST /households/{id}/members response — `{ message, member }`. */
@JsonClass(generateAdapter = true)
data class AddHouseholdMemberResponse(
    val message: String? = null,
    val member: HouseholdMember? = null,
)

/** POST/PUT /households body. */
@JsonClass(generateAdapter = true)
data class HouseholdInput(
    val name: String,
    val type: String,
)

/** POST /households/{id}/members body. */
@JsonClass(generateAdapter = true)
data class HouseholdMemberInput(
    @Json(name = "member_vcard_uid") val memberVCardUid: String,
    val role: String? = null,
    val since: String? = null,
    val until: String? = null,
)

/** POST /households/{id}/suggest-relationships response — newly created suggested edges. */
@JsonClass(generateAdapter = true)
data class SuggestRelationshipsResponse(
    val message: String? = null,
    @Json(name = "household_id") val householdId: String? = null,
    @Json(name = "suggested_edges") val suggestedEdges: List<RelationshipEdge> = emptyList(),
    val total: Int = 0,
)

/**
 * One T40 (shared-address) household suggestion. The (address_hash,
 * member_hash) pair is the stable identity the server recomputes on
 * accept/dismiss; member_vcard_uids is the suggested group.
 */
@JsonClass(generateAdapter = true)
data class AddressHouseholdSuggestion(
    @Json(name = "address_hash") val addressHash: String = "",
    @Json(name = "member_hash") val memberHash: String = "",
    @Json(name = "member_vcard_uids") val memberVCardUids: List<String> = emptyList(),
    val address: Address? = null,
)

/** POST /households/suggest-addresses response. */
@JsonClass(generateAdapter = true)
data class AddressSuggestionsResponse(
    val suggestions: List<AddressHouseholdSuggestion> = emptyList(),
    val total: Int = 0,
)

/** POST /households/suggestions/accept body. */
@JsonClass(generateAdapter = true)
data class AcceptHouseholdSuggestionInput(
    @Json(name = "member_vcard_uids") val memberVCardUids: List<String>,
    val name: String? = null,
    val type: String? = null,
)

/** POST /households/suggestions/accept response — wrapped `{ household }`, unwrapped in ApiClient. */
@JsonClass(generateAdapter = true)
data class AcceptHouseholdSuggestionResponse(
    val household: Household? = null,
)

/** POST /households/suggestions/dismiss body. */
@JsonClass(generateAdapter = true)
data class DismissHouseholdSuggestionInput(
    @Json(name = "member_vcard_uids") val memberVCardUids: List<String>,
)

/**
 * Renders a suggestion's address as a single display line (street, locality,
 * region, postcode, country), falling back to the full text when present.
 * Mirrors web's formatSuggestionAddress. The sub-street parts (PO box /
 * apartment / floor) are deliberately NOT rendered: a suggestion's address is
 * a *building*-level shared address (matching backend AddressNormalizedKey's
 * scope, which also excludes them).
 */
fun formatSuggestionAddress(address: Address?): String {
    if (address == null) return ""
    address.full?.takeIf { it.isNotBlank() }?.let { return it }
    val byKind: Map<String, String> = buildMap {
        address.components.orEmpty().forEach { comp ->
            if (comp.kind != null && !containsKey(comp.kind)) put(comp.kind, comp.value.orEmpty())
        }
    }
    return listOf("name", "locality", "region", "postcode", "country")
        .map { byKind[it].orEmpty().trim() }
        .filter { it.isNotEmpty() }
        .joinToString(", ")
}
