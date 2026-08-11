package com.mycorrhizal.crm.data.local

import androidx.room.Entity
import androidx.room.PrimaryKey

/** Local cache of one circle. Read-only mirror data (online-first). */
@Entity(tableName = "cached_circles")
data class CachedCircle(
    @PrimaryKey val id: String,
    val name: String = "",
    val updatedAt: String? = null,
)

/**
 * Local cache of one circle membership (join row). Mirrors the backend's
 * circle_members join — hard-delete semantics per CLAUDE.md (a join row *is*
 * its endpoints), so removeCircleMember deletes the row outright.
 */
@Entity(tableName = "cached_circle_members")
data class CachedCircleMember(
    @PrimaryKey val id: Int,
    val circleId: String,
    val memberVCardUid: String,
)
