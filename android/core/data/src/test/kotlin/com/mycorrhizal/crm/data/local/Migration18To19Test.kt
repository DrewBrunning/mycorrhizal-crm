package com.mycorrhizal.crm.data.local

import android.content.Context
import android.database.sqlite.SQLiteDatabase
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Occasions event planning (docs/adrs/0026-occasions-events.md, issue #1228):
 * [MIGRATION_18_19] adds `cached_occasion_events`, a new table only — no
 * existing table's schema changes. Same rationale as [Migration14To15Test] for
 * why this is hand-written rather than left to `fallbackToDestructiveMigration`:
 * any version bump that relies on the destructive path drops
 * `pending_interactions` (the not-yet-synced outbox), so this test's realistic
 * "before" database carries an outbox row through the hop too.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class Migration18To19Test {

    private lateinit var context: Context
    private lateinit var dbFile: java.io.File

    @Before
    fun setup() {
        context = ApplicationProvider.getApplicationContext()
        dbFile = context.getDatabasePath("migration-18-19-test.db")
        dbFile.delete()
    }

    @After
    fun teardown() {
        dbFile.delete()
    }

    private fun createV18Database() {
        val db = SQLiteDatabase.openOrCreateDatabase(dbFile, null)
        LocalDatabaseSchemaFixtures.createV18Tables(db)
        db.execSQL(
            "INSERT INTO cached_contacts (id, fn, primaryPhone, phonesNormalized, archived, deleted) " +
                "VALUES (1, 'Dana White', '(800) 555-1234', '8005551234 5551234', 0, 0)",
        )
        db.execSQL(
            "INSERT INTO pending_interactions (timestampMillis, kind, phoneNumber, synced) " +
                "VALUES (1000, 'call', '5551234', 0)",
        )
        db.version = 18
        db.close()
    }

    @Test
    fun `migration preserves outbox and contacts and adds occasion events`() = runBlocking {
        createV18Database()

        val db = Room.databaseBuilder(context, AppDatabase::class.java, dbFile.absolutePath)
            .addMigrations(*REGISTERED_MIGRATIONS.toTypedArray())
            .build()

        // Neither of these tables is touched by this migration — they must survive untouched.
        val pending = db.pendingInteractionDao().getUnsynced()
        assertEquals(1, pending.size)
        assertEquals("5551234", pending[0].phoneNumber)
        val contact = db.cachedContactDao().getById(1)
        assertEquals("Dana White", contact?.fn)

        // The new table exists and is fully usable through the DAO.
        db.cachedOccasionEventDao().upsert(
            CachedOccasionEvent(
                id = "event-1",
                title = "Summer BBQ",
                startsAt = "2026-07-04T15:00:00Z",
                sensitivity = "normal",
            ),
        )
        val events = db.cachedOccasionEventDao().getAll()
        assertEquals(1, events.size)
        assertEquals("Summer BBQ", events[0].title)

        db.close()
    }
}
