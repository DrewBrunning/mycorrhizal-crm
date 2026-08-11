package com.mycorrhizal.crm.data.local

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.Query

@Dao
interface PendingInteractionDao {
    @Insert
    suspend fun insert(interaction: PendingInteraction): Long

    @Query("SELECT * FROM pending_interactions WHERE synced = 0 ORDER BY timestampMillis ASC")
    suspend fun getUnsynced(): List<PendingInteraction>

    @Query("UPDATE pending_interactions SET synced = 1, syncedAt = :syncedAt WHERE id = :id")
    suspend fun markSynced(id: Long, syncedAt: String)

    @Query("SELECT COUNT(*) FROM pending_interactions WHERE synced = 0")
    suspend fun countUnsynced(): Int

    @Query("DELETE FROM pending_interactions WHERE synced = 1")
    suspend fun deleteSynced()

    @Query("SELECT * FROM pending_interactions WHERE kind = :kind AND phoneNumber = :phoneNumber AND timestampMillis > :sinceMillis ORDER BY timestampMillis DESC LIMIT 1")
    suspend fun findRecent(kind: String, phoneNumber: String, sinceMillis: Long): PendingInteraction?
}
