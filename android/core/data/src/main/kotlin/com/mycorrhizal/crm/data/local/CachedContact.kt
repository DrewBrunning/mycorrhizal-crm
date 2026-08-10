package com.mycorrhizal.crm.data.local

import androidx.room.Entity
import androidx.room.PrimaryKey

/**
 * Local cache of a contact's list-projection fields plus the full neutral
 * Card/CRM JSON captured from the detail endpoint. The cache is read-only
 * fallback data — writes always go to the server first (online-first).
 *
 * `cardJson`/`crmJson` hold the raw ContactRecordResponse.card / .crm
 * payloads so an offline detail screen can render the full Card without a
 * second network round-trip.
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
    val circlesJson: String? = null,
    val archived: Boolean = false,
    val deleted: Boolean = false,
    val cardJson: String? = null,
    val crmJson: String? = null,
    /** Server-side updated_at, used for cache freshness decisions. */
    val updatedAt: String? = null,
) {
    val displayName: String
        get() = fn?.takeIf { it.isNotBlank() }
            ?: listOfNotNull(firstname, lastname).joinToString(" ").ifBlank { "#$id" }
}
