package com.mycorrhizal.crm.data.local

import android.content.Context
import android.database.sqlite.SQLiteDatabase
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withContext
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issue #480 recommended action 4: `pending_interactions` is the one table in this database that
 * is a real not-yet-synced outbox rather than rebuildable cache data (`AppDatabase`'s doc
 * comment) — every registered migration exists specifically to keep the destructive fallback
 * away from it. [Migration13To14Test]/[Migration14To15Test]/[Migration15To16Test] each carry one
 * outbox row through their own single hop as a smoke check; this test is the dedicated, thorough
 * one the ticket asks for: every field combination the DAO/entity actually support, run through
 * the **whole** registered chain (v13 -> current) in one open, not just adjacent hops.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class PendingInteractionsSurviveMigrationTest {

    private lateinit var context: Context
    private lateinit var dbFile: java.io.File

    @Before
    fun setup() {
        context = ApplicationProvider.getApplicationContext()
        dbFile = context.getDatabasePath("pending-interactions-migration-test.db")
        dbFile.delete()
    }

    @After
    fun teardown() {
        dbFile.delete()
    }

    /**
     * Seeds a v13 outbox with one row per field combination the entity/DAO actually support:
     * a synced call with every optional field populated, an unsynced incoming call, an unsynced
     * missed call, and an unsynced message with no `direction` (messages don't have one) and no
     * `matchedContactId` (an unmatched number).
     */
    private fun createV13DatabaseWithFullOutbox() {
        val db = SQLiteDatabase.openOrCreateDatabase(dbFile, null)
        LocalDatabaseSchemaFixtures.createV13Tables(db)
        db.execSQL(
            "INSERT INTO cached_contacts (id, fn, primaryPhone, archived, deleted) " +
                "VALUES (1, 'Dana White', '(800) 555-1234', 0, 0)",
        )
        db.execSQL(
            "INSERT INTO pending_interactions " +
                "(timestampMillis, kind, direction, phoneNumber, matchedContactId, synced, syncedAt) " +
                "VALUES (1000, 'call', 'outgoing', '5551234', 1, 1, '2026-01-01T00:00:00Z')",
        )
        db.execSQL(
            "INSERT INTO pending_interactions " +
                "(timestampMillis, kind, direction, phoneNumber, matchedContactId, synced, syncedAt) " +
                "VALUES (2000, 'call', 'incoming', '5559999', NULL, 0, NULL)",
        )
        db.execSQL(
            "INSERT INTO pending_interactions " +
                "(timestampMillis, kind, direction, phoneNumber, matchedContactId, synced, syncedAt) " +
                "VALUES (3000, 'call', 'missed', '5558888', NULL, 0, NULL)",
        )
        db.execSQL(
            "INSERT INTO pending_interactions " +
                "(timestampMillis, kind, direction, phoneNumber, matchedContactId, synced, syncedAt) " +
                "VALUES (4000, 'message', NULL, '5557777', NULL, 0, NULL)",
        )
        db.version = 13
        db.close()
    }

    @Test
    fun `every outbox row and field survives the full v13 to current migration chain`() = runBlocking {
        createV13DatabaseWithFullOutbox()

        val db = Room.databaseBuilder(context, AppDatabase::class.java, dbFile.absolutePath)
            .addMigrations(*REGISTERED_MIGRATIONS.toTypedArray())
            .build()

        // countUnsynced/getUnsynced only see the three unsynced rows — the synced one is a real
        // row too (assertRawCount below), just outside this query's WHERE clause.
        assertEquals(3, db.pendingInteractionDao().countUnsynced())
        val unsynced = db.pendingInteractionDao().getUnsynced()
        assertEquals(3, unsynced.size)

        val incoming = unsynced.single { it.phoneNumber == "5559999" }
        assertEquals("call", incoming.kind)
        assertEquals("incoming", incoming.direction)
        assertNull(incoming.matchedContactId)
        assertTrue(!incoming.synced)

        val missed = unsynced.single { it.phoneNumber == "5558888" }
        assertEquals("missed", missed.direction)

        val message = unsynced.single { it.phoneNumber == "5557777" }
        assertEquals("message", message.kind)
        assertNull("messages carry no call direction", message.direction)
        assertNull(message.matchedContactId)

        // The already-synced row isn't in getUnsynced()'s result set by design (it queries
        // synced = 0), but it must still exist post-migration with every field intact — a
        // migration silently losing only the "already synced" rows would pass a naive check
        // that only ever calls getUnsynced().
        val allRows = withContext(Dispatchers.IO) {
            db.query("SELECT * FROM pending_interactions ORDER BY timestampMillis ASC", null)
        }
        allRows.use { cursor ->
            assertEquals(4, cursor.count)
            assertTrue(cursor.moveToFirst())
            assertEquals(1, cursor.getInt(cursor.getColumnIndexOrThrow("synced")))
            assertEquals("outgoing", cursor.getString(cursor.getColumnIndexOrThrow("direction")))
            assertEquals(1, cursor.getInt(cursor.getColumnIndexOrThrow("matchedContactId")))
            assertEquals("2026-01-01T00:00:00Z", cursor.getString(cursor.getColumnIndexOrThrow("syncedAt")))
        }

        // The matched contact itself also survived (a matchedContactId pointing at a row that
        // got wiped would be a silent dangling reference, not caught by the outbox count alone).
        assertEquals("Dana White", db.cachedContactDao().getById(1)?.fn)

        db.close()
    }
}
