package com.mycorrhizal.crm.data.local

import androidx.room.Entity
import androidx.room.PrimaryKey

/**
 * Local cache of one activity (Interaction). Read-only mirror data; the full
 * participant list is kept as a JSON blob since the detail screen only shows
 * titles and dates for now.
 */
@Entity(tableName = "cached_activities")
data class CachedActivity(
    @PrimaryKey val id: Int,
    val title: String? = null,
    val description: String? = null,
    val location: String? = null,
    val date: String? = null,
    val type: String? = null,
    val deleted: Boolean = false,
)
