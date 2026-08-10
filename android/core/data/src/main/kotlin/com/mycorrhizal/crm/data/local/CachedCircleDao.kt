package com.mycorrhizal.crm.data.local

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query

@Dao
interface CachedCircleDao {
    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsertAll(circles: List<CachedCircle>)

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(circle: CachedCircle)

    @Query("SELECT * FROM cached_circles ORDER BY name COLLATE NOCASE ASC")
    suspend fun getAll(): List<CachedCircle>

    @Query("SELECT * FROM cached_circles WHERE id = :id")
    suspend fun getById(id: String): CachedCircle?

    @Query("DELETE FROM cached_circles WHERE id = :id")
    suspend fun deleteById(id: String)

    @Query("DELETE FROM cached_circles")
    suspend fun deleteAll()
}

@Dao
interface CachedCircleMemberDao {
    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsertAll(members: List<CachedCircleMember>)

    @Query("SELECT * FROM cached_circle_members WHERE circleId = :circleId")
    suspend fun getByCircleId(circleId: String): List<CachedCircleMember>

    @Query("DELETE FROM cached_circle_members WHERE circleId = :circleId")
    suspend fun deleteByCircleId(circleId: String)

    @Query("DELETE FROM cached_circle_members WHERE circleId = :circleId AND memberVCardUid = :vcardUid")
    suspend fun deleteMember(circleId: String, vcardUid: String)
}
