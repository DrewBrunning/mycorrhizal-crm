package com.mycorrhizal.crm.data.local

import androidx.room.Database
import androidx.room.RoomDatabase
import androidx.room.TypeConverters

/**
 * Local cache database. This is a cache, not a source of truth — it mirrors
 * readable data for offline viewing and fast list rendering. Schema grows as
 * later phases mirror more tables; exportSchema is off because the cache can
 * always be rebuilt from the server (fallbackToDestructiveMigration).
 */
@Database(
    entities = [
        CachedContact::class,
        CachedContactFts::class,
        CachedActivity::class,
        CachedNote::class,
        CachedReminder::class,
        CachedCircle::class,
        CachedCircleMember::class,
        CachedTag::class,
        CachedContactTag::class,
        CachedHousehold::class,
        CachedHouseholdMember::class,
        CachedRelationshipEdge::class,
        CachedLifeEvent::class,
        CachedGift::class,
        CachedPreference::class,
        CachedConversationAgenda::class,
        PendingInteraction::class,
    ],
    version = 11,
    exportSchema = false,
)
@TypeConverters(Converters::class)
abstract class AppDatabase : RoomDatabase() {
    abstract fun cachedContactDao(): CachedContactDao
    abstract fun cachedActivityDao(): CachedActivityDao
    abstract fun cachedNoteDao(): CachedNoteDao
    abstract fun cachedReminderDao(): CachedReminderDao
    abstract fun cachedCircleDao(): CachedCircleDao
    abstract fun cachedCircleMemberDao(): CachedCircleMemberDao
    abstract fun cachedTagDao(): CachedTagDao
    abstract fun cachedContactTagDao(): CachedContactTagDao
    abstract fun cachedHouseholdDao(): CachedHouseholdDao
    abstract fun cachedHouseholdMemberDao(): CachedHouseholdMemberDao
    abstract fun cachedRelationshipEdgeDao(): CachedRelationshipEdgeDao
    abstract fun cachedLifeEventDao(): CachedLifeEventDao
    abstract fun cachedGiftDao(): CachedGiftDao
    abstract fun cachedPreferenceDao(): CachedPreferenceDao
    abstract fun cachedConversationAgendaDao(): CachedConversationAgendaDao
    abstract fun pendingInteractionDao(): PendingInteractionDao
}
