package com.mycorrhizal.crm.data.local

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query

@Dao
interface CachedRelationshipEdgeDao {
    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsertAll(edges: List<CachedRelationshipEdge>)

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(edge: CachedRelationshipEdge)

    @Query("SELECT * FROM cached_relationship_edges WHERE sourceId = :uid OR targetId = :uid ORDER BY updatedAt DESC")
    suspend fun getForContact(uid: String): List<CachedRelationshipEdge>

    @Query("SELECT * FROM cached_relationship_edges WHERE id = :id")
    suspend fun getById(id: String): CachedRelationshipEdge?

    @Query("DELETE FROM cached_relationship_edges WHERE id = :id")
    suspend fun deleteById(id: String)

    @Query("DELETE FROM cached_relationship_edges")
    suspend fun deleteAll()
}
