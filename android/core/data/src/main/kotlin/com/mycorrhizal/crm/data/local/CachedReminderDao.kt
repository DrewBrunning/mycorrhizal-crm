package com.mycorrhizal.crm.data.local

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query

@Dao
interface CachedReminderDao {
    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsertAll(reminders: List<CachedReminder>)

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(reminder: CachedReminder)

    @Query("SELECT * FROM cached_reminders ORDER BY remindAt ASC")
    suspend fun getAll(): List<CachedReminder>

    @Query("SELECT * FROM cached_reminders WHERE id = :id")
    suspend fun getById(id: Int): CachedReminder?

    @Query("DELETE FROM cached_reminders")
    suspend fun deleteAll()
}
