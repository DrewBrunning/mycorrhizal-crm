package com.mycorrhizal.crm.data.local

import androidx.room.Entity
import androidx.room.Fts4

/**
 * FTS4 mirror of [CachedContact] for fast local full-text search over the
 * cached contact list (Phase 2 item 13). Room generates the external-content
 * triggers that keep it in sync with `cached_contacts` on every insert/update/
 * delete, so a search over cached rows is a single FTS MATCH query — much
 * faster than LIKE scans, and it works offline.
 *
 * FTS4 (not FTS5) is used because Room's `@Fts4(contentEntity=...)` keeps the
 * virtual table in lockstep with the content table automatically; the FTS5
 * equivalent requires manual trigger management.
 */
@Fts4(contentEntity = CachedContact::class)
@Entity(tableName = "cached_contacts_fts")
data class CachedContactFts(
    val fn: String? = null,
    val firstname: String? = null,
    val lastname: String? = null,
    val primaryEmail: String? = null,
    val primaryPhone: String? = null,
    /** See [CachedContact.phonesNormalized]; T76 indexes this instead of the raw
     *  [primaryPhone] for phone-shaped queries, since punctuation splits the raw
     *  column into unmatchable FTS4 tokens. */
    val phonesNormalized: String? = null,
    val org: String? = null,
)
