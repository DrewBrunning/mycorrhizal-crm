package com.mycorrhizal.crm.data.local

import androidx.room.Entity
import androidx.room.PrimaryKey

/** Local cache of one life event. Read-only mirror data (online-first). */
@Entity(tableName = "cached_life_events")
data class CachedLifeEvent(
    @PrimaryKey val id: String,
    val entityId: String,
    val type: String? = null,
    val category: String? = null,
    val date: String? = null,
    val description: String? = null,
    val remind: Boolean? = null,
    val updatedAt: String? = null,
    val deleted: Boolean = false,
)

/** Local cache of one gift. */
@Entity(tableName = "cached_gifts")
data class CachedGift(
    @PrimaryKey val id: String,
    val entityId: String,
    val status: String = "idea",
    val occasion: String? = null,
    val description: String = "",
    val updatedAt: String? = null,
    val deleted: Boolean = false,
)

/** Local cache of one preference. */
@Entity(tableName = "cached_preferences")
data class CachedPreference(
    @PrimaryKey val id: String,
    val entityId: String,
    val category: String = "",
    val key: String? = null,
    val value: String = "",
    val sensitivity: String = "normal",
    val updatedAt: String? = null,
    val deleted: Boolean = false,
)

/** Local cache of one conversation-agenda item. */
@Entity(tableName = "cached_conversation_agenda")
data class CachedConversationAgenda(
    @PrimaryKey val id: String,
    val entityId: String,
    val content: String = "",
    val referenceUrl: String? = null,
    val discussedAt: String? = null,
    val updatedAt: String? = null,
    val deleted: Boolean = false,
)
