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
 * Issue #2629: [MIGRATION_19_20] adds `description` and `color` to `cached_circles` so the saved
 * copy of the circle list can be shown faithfully when the server is unreachable. Hand-written
 * (not the destructive fallback) for the same reason as every migration since 13: the
 * destructive path drops `pending_interactions`, the not-yet-synced outbox, so this test's
 * "before" database carries an outbox row and a circle through the hop.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class Migration19To20Test {

    private lateinit var context: Context
    private lateinit var dbFile: java.io.File

    @Before
    fun setup() {
        context = ApplicationProvider.getApplicationContext()
        dbFile = context.getDatabasePath("migration-19-20-test.db")
        dbFile.delete()
    }

    @After
    fun teardown() {
        dbFile.delete()
    }

    /**
     * A populated v19 database, as a user on the latest release (v1.6.0, Room v19) has it: several
     * circles (one with no `updatedAt`, one with a non-ASCII name), their memberships, a contact,
     * and a not-yet-synced outbox row. Built from the v19 shape (identical to the committed
     * `19.json`, pinned by [SchemaFixtureMatchesDumpTest]).
     */
    private fun createV19Database() {
        val db = SQLiteDatabase.openOrCreateDatabase(dbFile, null)
        LocalDatabaseSchemaFixtures.createV19Tables(db)
        db.execSQL(
            "INSERT INTO cached_circles (id, name, updatedAt) VALUES " +
                "('c1', 'Friends', '2026-01-01'), ('c2', 'Familia García', NULL), " +
                "('c3', 'Work', '2026-03-03T10:00:00Z')"
        )
        db.execSQL(
            "INSERT INTO cached_circle_members (id, circleId, memberVCardUid) VALUES " +
                "(1, 'c1', 'uid-a'), (2, 'c2', 'uid-b')"
        )
        db.execSQL(
            "INSERT INTO cached_contacts (id, fn, primaryPhone, phonesNormalized, archived, deleted) " +
                "VALUES (1, 'Dana White', '(800) 555-1234', '8005551234 5551234', 0, 0)"
        )
        db.execSQL(
            "INSERT INTO pending_interactions (timestampMillis, kind, phoneNumber, synced) " +
                "VALUES (1000, 'call', '5551234', 0)"
        )
        db.version = 19
        db.close()
    }

    private fun migrate(): AppDatabase = Room
        .databaseBuilder(context, AppDatabase::class.java, dbFile.absolutePath)
        .addMigrations(*REGISTERED_MIGRATIONS.toTypedArray())
        .build()

    @Test
    fun `migration keeps every circle with all old columns and gives new columns empty defaults`() =
        runBlocking {
            createV19Database()

            val db = migrate()

            val circles = db.cachedCircleDao().getAll().associateBy { it.id }
            assertEquals(setOf("c1", "c2", "c3"), circles.keys)
            assertEquals(
                CachedCircle("c1", "Friends", "2026-01-01", description = "", color = ""),
                circles["c1"]
            )
            assertEquals(
                CachedCircle("c2", "Familia García", null, description = "", color = ""),
                circles["c2"]
            )
            assertEquals(
                CachedCircle("c3", "Work", "2026-03-03T10:00:00Z", description = "", color = ""),
                circles["c3"]
            )
            db.close()
        }

    @Test
    fun `migration leaves the other tables and the outbox untouched`() = runBlocking {
        createV19Database()

        val db = migrate()

        val pending = db.pendingInteractionDao().getUnsynced()
        assertEquals(1, pending.size)
        assertEquals("5551234", pending[0].phoneNumber)
        assertEquals("Dana White", db.cachedContactDao().getById(1)?.fn)
        assertEquals(
            listOf("uid-a"),
            db.cachedCircleMemberDao().getByCircleId("c1").map { it.memberVCardUid }
        )
        assertEquals(
            listOf("uid-b"),
            db.cachedCircleMemberDao().getByCircleId("c2").map { it.memberVCardUid }
        )
        db.close()
    }

    @Test
    fun `migrated circles accept new values and old rows can be refreshed in place`() =
        runBlocking {
            createV19Database()

            val db = migrate()

            db.cachedCircleDao().upsert(
                CachedCircle(id = "c1", name = "Friends", description = "Close", color = "teal")
            )
            db.cachedCircleDao().upsert(
                CachedCircle(id = "c4", name = "New", description = "d", color = "red")
            )
            assertEquals("teal", db.cachedCircleDao().getById("c1")?.color)
            assertEquals("Close", db.cachedCircleDao().getById("c1")?.description)
            assertEquals("red", db.cachedCircleDao().getById("c4")?.color)
            assertEquals(4, db.cachedCircleDao().getAll().size)
            db.close()
        }
}
