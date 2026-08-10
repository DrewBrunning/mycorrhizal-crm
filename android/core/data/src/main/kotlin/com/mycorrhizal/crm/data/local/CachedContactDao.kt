package com.mycorrhizal.crm.data.local

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query

@Dao
interface CachedContactDao {
    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsertAll(contacts: List<CachedContact>)

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(contact: CachedContact)

    @Query("SELECT * FROM cached_contacts WHERE id = :id")
    suspend fun getById(id: Int): CachedContact?

    @Query("SELECT * FROM cached_contacts WHERE id IN (:ids)")
    suspend fun getByIds(ids: List<Int>): List<CachedContact>

    @Query("SELECT * FROM cached_contacts WHERE deleted = 0 ORDER BY fn COLLATE NOCASE ASC")
    suspend fun getAll(): List<CachedContact>

    @Query("SELECT * FROM cached_contacts WHERE deleted = 1")
    suspend fun getDeleted(): List<CachedContact>

    @Query(
        """
        SELECT * FROM cached_contacts
        WHERE deleted = 0
          AND (
              fn LIKE '%' || :query || '%'
              OR firstname LIKE '%' || :query || '%'
              OR lastname LIKE '%' || :query || '%'
              OR primaryEmail LIKE '%' || :query || '%'
              OR primaryPhone LIKE '%' || :query || '%'
          )
        ORDER BY fn COLLATE NOCASE ASC
        """,
    )
    suspend fun search(query: String): List<CachedContact>

    @Query("DELETE FROM cached_contacts")
    suspend fun deleteAll()

    @Query("DELETE FROM cached_contacts WHERE id IN (:ids)")
    suspend fun deleteByIds(ids: List<Int>)
}
