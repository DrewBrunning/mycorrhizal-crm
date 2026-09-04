package com.mycorrhizal.crm.data.local

import android.content.Context
import android.database.sqlite.SQLiteDatabase
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issue #480 recommended action 5: `fallbackToDestructiveMigration` (`DataModule`) is a
 * deliberate part of this database's design for the version pairs [MigrationVersionCoverageTest]
 * doesn't require a registered migration for — it must fire (and recover cleanly) for those, and
 * only those.
 *
 * [Migration13To14Test]/[Migration14To15Test]/[Migration15To16Test]/
 * [PendingInteractionsSurviveMigrationTest] already prove the negative for every version pair
 * this database has ever shipped: no destructive wipe happens across 13->14->15->16, because each
 * of those hops has a registered [Migration]. This test proves the positive: an old,
 * unrecognized schema — the shape of "a device on some ancient version this codebase has no
 * migration path from, or has never even heard of" — does not crash the app or hang; Room
 * destructively recreates the current schema, and the app carries on exactly like a fresh
 * install (the design's whole premise: everything except `pending_interactions` is a rebuildable
 * cache, so a wipe there is a recoverable, not a fatal, event).
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class DestructiveFallbackTest {

    private lateinit var context: Context
    private lateinit var dbFile: java.io.File

    @Before
    fun setup() {
        context = ApplicationProvider.getApplicationContext()
        dbFile = context.getDatabasePath("destructive-fallback-test.db")
        dbFile.delete()
    }

    @After
    fun teardown() {
        dbFile.delete()
    }

    /**
     * A schema this codebase has never registered a migration path from: no table this
     * database recognizes, at a version number below [EARLIEST_KNOWN_VERSION]. Stands in for
     * "install predates anything this app's migration chain accounts for" — the only case
     * [MigrationVersionCoverageTest] leaves to `fallbackToDestructiveMigration` on purpose.
     */
    private fun createUnrecognizedLegacyDatabase() {
        val db = SQLiteDatabase.openOrCreateDatabase(dbFile, null)
        db.execSQL("CREATE TABLE some_ancient_prototype_table (id INTEGER PRIMARY KEY, note TEXT)")
        db.execSQL("INSERT INTO some_ancient_prototype_table (note) VALUES ('pre-migration-chain data')")
        db.version = EARLIEST_KNOWN_VERSION - 5
        db.close()
    }

    @Test
    fun `an unrecognized version gap triggers destructive recreate and the app recovers`() = runBlocking {
        createUnrecognizedLegacyDatabase()

        // The exact same builder configuration DataModule uses: the real registered migrations
        // plus the destructive fallback, so this proves what production actually does when it
        // meets a gap the migration chain doesn't cover — not a hypothetical.
        val db = Room.databaseBuilder(context, AppDatabase::class.java, dbFile.absolutePath)
            .addMigrations(*REGISTERED_MIGRATIONS.toTypedArray())
            .fallbackToDestructiveMigration()
            .build()

        // Opens without throwing (no crash, no hang) and the current schema exists — the
        // recognized tables from the unrecognized legacy shape are simply gone, which is the
        // destructive recreate actually having happened rather than some other recovery path.
        assertEquals(0, db.cachedContactDao().getAll().size)
        assertEquals(0, db.pendingInteractionDao().countUnsynced())

        // The app functions exactly as it would post-fresh-install: the cache rebuilds from the
        // server (simulated here by writing through the DAOs) as soon as the next sync runs.
        db.cachedContactDao().upsert(CachedContact(id = 1, fn = "Rebuilt After Wipe"))
        assertEquals("Rebuilt After Wipe", db.cachedContactDao().getById(1)?.fn)
        db.cachedCadencePolicyDao().upsert(
            CachedCadencePolicy(id = "policy-1", entityId = "contact-1", targetIntervalDays = 14),
        )
        assertEquals(1, db.cachedCadencePolicyDao().getForContact("contact-1").size)

        db.close()
    }

    @Test
    fun `destructive fallback does not fire when every hop has a registered migration`() = runBlocking {
        // The real, ever-shipped v13 shape with outbox data — every hop from here to current is
        // covered by REGISTERED_MIGRATIONS (MigrationVersionCoverageTest pins that), so this must
        // go through the migration chain, not the destructive path.
        val db2 = SQLiteDatabase.openOrCreateDatabase(dbFile, null)
        LocalDatabaseSchemaFixtures.createV13Tables(db2)
        db2.execSQL(
            "INSERT INTO pending_interactions (timestampMillis, kind, phoneNumber, synced) " +
                "VALUES (1000, 'call', '5551234', 0)",
        )
        db2.version = 13
        db2.close()

        val db = Room.databaseBuilder(context, AppDatabase::class.java, dbFile.absolutePath)
            .addMigrations(*REGISTERED_MIGRATIONS.toTypedArray())
            .fallbackToDestructiveMigration()
            .build()

        // If the destructive path had fired instead of the migration chain, this outbox row —
        // present only because the real 13->16 migration chain preserves it — would be gone.
        assertTrue(db.pendingInteractionDao().countUnsynced() == 1)

        db.close()
    }
}
