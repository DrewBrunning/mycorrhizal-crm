package com.mycorrhizal.crm.data.local

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query

@Dao
interface CachedTagDao {
    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsertAll(tags: List<CachedTag>)

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(tag: CachedTag)

    @Query("SELECT * FROM cached_tags ORDER BY name COLLATE NOCASE ASC")
    suspend fun getAll(): List<CachedTag>

    @Query("SELECT * FROM cached_tags WHERE id = :id")
    suspend fun getById(id: String): CachedTag?

    @Query("DELETE FROM cached_tags WHERE id = :id")
    suspend fun deleteById(id: String)

    @Query("DELETE FROM cached_tags")
    suspend fun deleteAll()
}

@Dao
interface CachedContactTagDao {
    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsertAll(taggings: List<CachedContactTag>)

    @Query("SELECT * FROM cached_contact_tags WHERE tagId = :tagId")
    suspend fun getByTagId(tagId: String): List<CachedContactTag>

    @Query("DELETE FROM cached_contact_tags WHERE tagId = :tagId")
    suspend fun deleteByTagId(tagId: String)

    @Query("DELETE FROM cached_contact_tags WHERE tagId = :tagId AND contactVCardUid = :vcardUid")
    suspend fun deleteTagging(tagId: String, vcardUid: String)

    @Query("DELETE FROM cached_contact_tags")
    suspend fun deleteAll()
}
