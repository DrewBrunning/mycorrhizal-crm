package com.mycorrhizal.crm.data.local

import androidx.room.Entity
import androidx.room.PrimaryKey

/** Local cache of one household. Read-only mirror data (online-first). */
@Entity(tableName = "cached_households")
data class CachedHousehold(
    @PrimaryKey val id: String,
    val name: String = "",
    val type: String = "family_unit",
    val updatedAt: String? = null,
)

/**
 * Local cache of one household membership (join row). Mirrors the backend's
 * household_members join — hard-delete semantics per CLAUDE.md.
 */
@Entity(tableName = "cached_household_members")
data class CachedHouseholdMember(
    @PrimaryKey val id: Int,
    val householdId: String,
    val memberVCardUid: String,
    val role: String? = null,
    val since: String? = null,
    val until: String? = null,
)
