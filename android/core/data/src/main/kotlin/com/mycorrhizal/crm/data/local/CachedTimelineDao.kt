package com.mycorrhizal.crm.data.local

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query

@Dao
interface CachedLifeEventDao {
    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsertAll(items: List<CachedLifeEvent>)

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(item: CachedLifeEvent)

    @Query("SELECT * FROM cached_life_events WHERE entityId = :entityId ORDER BY updatedAt DESC")
    suspend fun getForContact(entityId: String): List<CachedLifeEvent>

    @Query("DELETE FROM cached_life_events WHERE id = :id")
    suspend fun deleteById(id: String)

    @Query("DELETE FROM cached_life_events WHERE entityId = :entityId")
    suspend fun deleteForContact(entityId: String)

    @Query("DELETE FROM cached_life_events")
    suspend fun deleteAll()
}

@Dao
interface CachedGiftDao {
    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsertAll(items: List<CachedGift>)

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(item: CachedGift)

    @Query("SELECT * FROM cached_gifts WHERE entityId = :entityId ORDER BY updatedAt DESC")
    suspend fun getForContact(entityId: String): List<CachedGift>

    @Query("DELETE FROM cached_gifts WHERE id = :id")
    suspend fun deleteById(id: String)

    @Query("DELETE FROM cached_gifts WHERE entityId = :entityId")
    suspend fun deleteForContact(entityId: String)

    @Query("DELETE FROM cached_gifts")
    suspend fun deleteAll()
}

@Dao
interface CachedPreferenceDao {
    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsertAll(items: List<CachedPreference>)

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(item: CachedPreference)

    @Query("SELECT * FROM cached_preferences WHERE entityId = :entityId ORDER BY updatedAt DESC")
    suspend fun getForContact(entityId: String): List<CachedPreference>

    @Query("DELETE FROM cached_preferences WHERE id = :id")
    suspend fun deleteById(id: String)

    @Query("DELETE FROM cached_preferences WHERE entityId = :entityId")
    suspend fun deleteForContact(entityId: String)

    @Query("DELETE FROM cached_preferences")
    suspend fun deleteAll()
}

@Dao
interface CachedConversationAgendaDao {
    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsertAll(items: List<CachedConversationAgenda>)

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(item: CachedConversationAgenda)

    @Query("SELECT * FROM cached_conversation_agenda WHERE entityId = :entityId ORDER BY updatedAt DESC")
    suspend fun getForContact(entityId: String): List<CachedConversationAgenda>

    @Query("DELETE FROM cached_conversation_agenda WHERE id = :id")
    suspend fun deleteById(id: String)

    @Query("DELETE FROM cached_conversation_agenda WHERE entityId = :entityId")
    suspend fun deleteForContact(entityId: String)

    @Query("DELETE FROM cached_conversation_agenda")
    suspend fun deleteAll()
}
