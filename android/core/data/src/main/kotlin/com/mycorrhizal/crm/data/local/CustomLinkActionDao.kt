package com.mycorrhizal.crm.data.local

import androidx.room.Dao
import androidx.room.Delete
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query

@Dao
interface CustomLinkActionDao {
    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(action: CustomLinkAction): Long

    @Query("SELECT * FROM custom_link_actions WHERE protocol = :protocol ORDER BY id ASC")
    suspend fun getForProtocol(protocol: String): List<CustomLinkAction>

    @Query("SELECT * FROM custom_link_actions ORDER BY protocol ASC, id ASC")
    suspend fun getAll(): List<CustomLinkAction>

    @Delete
    suspend fun delete(action: CustomLinkAction)
}
