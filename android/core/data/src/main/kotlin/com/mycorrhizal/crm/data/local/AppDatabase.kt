package com.mycorrhizal.crm.data.local

import androidx.room.Database
import androidx.room.RoomDatabase
import androidx.room.TypeConverters

/**
 * Local cache database. This is a cache, not a source of truth — it mirrors
 * readable data for offline viewing and fast list rendering. Schema grows as
 * later phases mirror more tables. For most tables that makes
 * `fallbackToDestructiveMigration` an acceptable general policy — a wipe just
 * means the cache rebuilds from the server on next sync.
 *
 * **Exception: `pending_interactions`** is a real not-yet-synced outbox (queued
 * call/SMS tracking), not rebuildable cache data — a version bump touching its
 * schema must not rely on the destructive fallback. See [MIGRATION_13_14] (T76)
 * for the pattern: a hand-written `Migration` registered in [REGISTERED_MIGRATIONS]
 * (`Migrations.kt`), scoped to only the tables that actually changed.
 * [MigrationVersionCoverageTest] is the guard that a future version bump can't
 * silently skip this — it fails CI if a version pair between
 * [EARLIEST_KNOWN_VERSION] and [CURRENT_VERSION] has neither a registered migration nor
 * an explicit accepted-destructive entry.
 *
 * **`exportSchema` (issue #480):** on, as of this database's version 16. Room
 * writes the schema JSON `MigrationTestHelper` needs into `core/data/schemas/`
 * on every compile (wired via `room.schemaLocation` in this module's
 * `build.gradle.kts`) — see the `Migration*Test` classes for how it's used.
 * It was off from this table's introduction through version 16, on the belief
 * that "the cache can always be rebuilt from the server" — true for every table
 * except `pending_interactions`, which is exactly why that belief didn't cover
 * this database as a whole. There is no schema JSON for any version below 16:
 * turning the flag on doesn't retroactively produce historical exports, and
 * this repo's git history shows this file's very first commit (the M1 Android
 * app, PR #80, 2026-08-10) already declared `version = 13` — no release of
 * this app ever shipped a lower version, so there is no real device schema
 * below 13 to reconstruct or protect. Versions 13-16 (every migration that
 * predates this ticket) are covered by hand-built "before" databases in the
 * `Migration*Test` classes instead — those tests construct the pre-migration
 * DDL directly and drive the real migration through Room's actual database
 * builder (which still runs `onValidateSchema` against the live entity
 * annotations regardless of `exportSchema`), so they carry the same
 * regression protection MigrationTestHelper would; they just can't use
 * MigrationTestHelper's JSON-backed `createDatabase` convenience. Any
 * migration registered from here on (17+) has a real prior-version JSON to
 * test against and should use `MigrationTestHelper` directly.
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
        CachedCadencePolicy::class,
        PendingInteraction::class,
        CustomLinkAction::class,
    ],
    version = CURRENT_VERSION,
    exportSchema = true,
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
    abstract fun cachedCadencePolicyDao(): CachedCadencePolicyDao
    abstract fun pendingInteractionDao(): PendingInteractionDao
    abstract fun customLinkActionDao(): CustomLinkActionDao
}
