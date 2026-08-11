package com.mycorrhizal.crm.data.local

import androidx.room.Entity
import androidx.room.PrimaryKey
import com.mycorrhizal.crm.model.network.CRMEnvelope
import com.mycorrhizal.crm.model.network.Card

/**
 * Local cache of a contact's list-projection fields plus the full neutral
 * Card/CRM for the detail screen. The cache is read-only fallback data —
 * writes always go to the server first (online-first). `card` and `crm`
 * hold the nested models, JSON-encoded by [Converters].
 */
@Entity(tableName = "cached_contacts")
data class CachedContact(
    @PrimaryKey val id: Int,
    val uid: String? = null,
    val firstname: String? = null,
    val lastname: String? = null,
    val nickname: String? = null,
    val fn: String? = null,
    val primaryEmail: String? = null,
    val primaryPhone: String? = null,
    val birthday: String? = null,
    val org: String? = null,
    val photoThumbnail: String? = null,
    val circles: List<String>? = null,
    val archived: Boolean = false,
    val deleted: Boolean = false,
    /** Device Contacts LOOKUP_KEY after a T57 import (§7.5.4); null otherwise. */
    val deviceLookupKey: String? = null,
    val card: Card? = null,
    val crm: CRMEnvelope? = null,
    /** Server-side updated_at, used for cache freshness decisions. */
    val updatedAt: String? = null,
) {
    val displayName: String
        get() = fn?.takeIf { it.isNotBlank() }
            ?: listOfNotNull(firstname, lastname).joinToString(" ").ifBlank { "#$id" }
}
