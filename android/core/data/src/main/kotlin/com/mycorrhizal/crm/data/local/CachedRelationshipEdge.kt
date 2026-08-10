package com.mycorrhizal.crm.data.local

import androidx.room.Entity
import androidx.room.PrimaryKey

/**
 * Local cache of one relationship edge. Mirrors the backend's
 * relationship_edges table. Edges are user-authored content (soft delete
 * server-side per CLAUDE.md), so the cache keeps a `deleted` tombstone flag
 * set from the `?since=` change feed; the browse list replaces rows outright.
 */
@Entity(tableName = "cached_relationship_edges")
data class CachedRelationshipEdge(
    @PrimaryKey val id: String,
    val sourceId: String,
    val targetId: String,
    val type: String,
    val directional: Boolean = false,
    val status: String = "confirmed",
    val sensitivity: String = "normal",
    val updatedAt: String? = null,
    val deleted: Boolean = false,
)
