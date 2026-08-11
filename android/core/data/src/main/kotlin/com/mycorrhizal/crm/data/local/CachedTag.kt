package com.mycorrhizal.crm.data.local

import androidx.room.Entity
import androidx.room.PrimaryKey

/** Local cache of one tag. Read-only mirror data (online-first). */
@Entity(tableName = "cached_tags")
data class CachedTag(
    @PrimaryKey val id: String,
    val name: String = "",
    val updatedAt: String? = null,
)

/**
 * Local cache of one contact-tagging (join row). Mirrors the backend's
 * contact_tags join — hard-delete semantics per CLAUDE.md.
 */
@Entity(tableName = "cached_contact_tags")
data class CachedContactTag(
    @PrimaryKey val id: Int,
    val tagId: String,
    val contactVCardUid: String,
)
