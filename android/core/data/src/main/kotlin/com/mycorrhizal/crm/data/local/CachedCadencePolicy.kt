package com.mycorrhizal.crm.data.local

import androidx.room.Entity
import androidx.room.PrimaryKey

/** Local cache of one cadence policy. Read-only mirror data (online-first). */
@Entity(tableName = "cached_cadence_policies")
data class CachedCadencePolicy(
    @PrimaryKey val id: String,
    val entityId: String,
    val targetIntervalDays: Int = 0,
    val qualifyingTypes: List<String> = emptyList(),
    val updatedAt: String? = null,
    val deleted: Boolean = false,
)
