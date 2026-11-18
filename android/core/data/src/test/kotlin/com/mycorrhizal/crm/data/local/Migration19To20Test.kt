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

    private fun createV19Database() {
        val db = SQLiteDatabase.openOrCreateDatabase(dbFile, null)
        LocalDatabaseSchemaFixtures.createV19Tables(db)
        db.execSQL(
            "INSERT INTO cached_circles (id, name, updatedAt) VALUES ('c1', 'Friends', '2026-01-01')"
        )
        db.execSQL(
            "INSERT INTO pending_interactions (timestampMillis, kind, phoneNumber, synced) " +
                "VALUES (1000, 'call', '5551234', 0)"
        )
        db.version = 19
        db.close()
    }

    @Test
    fun `migration keeps the outbox and existing circles and adds empty description and colour`() =
        runBlocking {
            createV19Database()

            val db = Room.databaseBuilder(context, AppDatabase::class.java, dbFile.absolutePath)
                .addMigrations(*REGISTERED_MIGRATIONS.toTypedArray())
                .build()

            val pending = db.pendingInteractionDao().getUnsynced()
            assertEquals(1, pending.size)
            assertEquals("5551234", pending[0].phoneNumber)

            val circle = db.cachedCircleDao().getById("c1")
            assertEquals("Friends", circle?.name)
            assertEquals("", circle?.description)
            assertEquals("", circle?.color)

            db.cachedCircleDao().upsert(
                CachedCircle(id = "c2", name = "Family", description = "Close", color = "teal")
            )
            assertEquals("teal", db.cachedCircleDao().getById("c2")?.color)

            db.close()
        }
}
