package com.mycorrhizal.crm.data.local

import androidx.room.Dao
import androidx.room.Entity
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.PrimaryKey
import androidx.room.Query

/**
 * Local cache of one occasion event (docs/adrs/0026-occasions-events.md, issue
 * #1228). Read-only mirror data (online-first): the event list is a full-resync
 * replacement, exactly like the timeline entities. Attendees are NOT cached —
 * they're fetched with the event detail on demand.
 */
@Entity(tableName = "cached_occasion_events")
data class CachedOccasionEvent(
    @PrimaryKey val id: String,
    val title: String = "",
    val startsAt: String = "",
    val endsAt: String? = null,
    val location: String? = null,
    val sensitivity: String = "normal",
    val notes: String? = null,
    val updatedAt: String? = null,
    val deleted: Boolean = false,
)

@Dao
interface CachedOccasionEventDao {
    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsertAll(items: List<CachedOccasionEvent>)

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(item: CachedOccasionEvent)

    @Query("SELECT * FROM cached_occasion_events ORDER BY startsAt ASC")
    suspend fun getAll(): List<CachedOccasionEvent>

    @Query("DELETE FROM cached_occasion_events WHERE id = :id")
    suspend fun deleteById(id: String)

    @Query("DELETE FROM cached_occasion_events")
    suspend fun deleteAll()
}
