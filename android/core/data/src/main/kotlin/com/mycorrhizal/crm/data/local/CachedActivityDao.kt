package com.mycorrhizal.crm.data.local

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query

@Dao
interface CachedActivityDao {
    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsertAll(activities: List<CachedActivity>)

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(activity: CachedActivity)

    @Query("SELECT * FROM cached_activities WHERE deleted = 0 ORDER BY date DESC")
    suspend fun getAll(): List<CachedActivity>

    @Query("SELECT * FROM cached_activities WHERE id = :id")
    suspend fun getById(id: Int): CachedActivity?

    @Query("DELETE FROM cached_activities")
    suspend fun deleteAll()
}
