package com.mycorrhizal.crm.data.local

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query

@Dao
interface CachedNoteDao {
    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsertAll(notes: List<CachedNote>)

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(note: CachedNote)

    @Query("SELECT * FROM cached_notes WHERE deleted = 0 ORDER BY date DESC")
    suspend fun getAll(): List<CachedNote>

    @Query("SELECT * FROM cached_notes WHERE id = :id")
    suspend fun getById(id: Int): CachedNote?

    /** M19: drop the local row after a successful server-side soft delete. */
    @Query("DELETE FROM cached_notes WHERE id = :id")
    suspend fun deleteById(id: Int)

    @Query("DELETE FROM cached_notes")
    suspend fun deleteAll()
}
