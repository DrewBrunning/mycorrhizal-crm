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
