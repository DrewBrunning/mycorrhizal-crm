package com.mycorrhizal.crm.data.local

import androidx.room.Entity
import androidx.room.PrimaryKey

/** Local cache of one reminder. Read-only mirror data. */
@Entity(tableName = "cached_reminders")
data class CachedReminder(
    @PrimaryKey val id: Int,
    val message: String? = null,
    val remindAt: String? = null,
    val recurrence: String? = null,
    val completed: Boolean = false,
)
