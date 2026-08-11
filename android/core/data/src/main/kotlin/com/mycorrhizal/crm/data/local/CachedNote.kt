package com.mycorrhizal.crm.data.local

import androidx.room.Entity
import androidx.room.PrimaryKey

/** Local cache of one note. Read-only mirror data. */
@Entity(tableName = "cached_notes")
data class CachedNote(
    @PrimaryKey val id: Int,
    val content: String? = null,
    val date: String? = null,
    val deleted: Boolean = false,
)
