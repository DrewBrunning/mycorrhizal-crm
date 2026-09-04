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
 * M12: [MIGRATION_14_15] adds `cached_cadence_policies`, a new table only — no existing table's
 * schema changes. Same rationale as [Migration13To14Test] for why this is hand-written rather
 * than left to `fallbackToDestructiveMigration`: any version bump that relies on the destructive
 * path drops `pending_interactions` (the not-yet-synced outbox) along with everything else, so
 * this test's realistic "before" database carries an outbox row through the hop too, even though
 * this particular migration's own SQL never touches that table.
 *
 * Issue #385 (SQLCipher): runs against the plain framework SQLite factory, same caveat as
 * [Migration13To14Test] — see that class's doc comment.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class Migration14To15Test {

    private lateinit var context: Context
    private lateinit var dbFile: java.io.File

    @Before
    fun setup() {
        context = ApplicationProvider.getApplicationContext()
        dbFile = context.getDatabasePath("migration-14-15-test.db")
        dbFile.delete()
    }

    @After
    fun teardown() {
        dbFile.delete()
    }

    private fun createV14Database() {
        val db = SQLiteDatabase.openOrCreateDatabase(dbFile, null)
        LocalDatabaseSchemaFixtures.createV14Tables(db)
        db.execSQL(
            "INSERT INTO cached_contacts (id, fn, primaryPhone, phonesNormalized, archived, deleted) " +
                "VALUES (1, 'Dana White', '(800) 555-1234', '8005551234 5551234', 0, 0)",
        )
        db.execSQL(
            "INSERT INTO pending_interactions (timestampMillis, kind, phoneNumber, synced) " +
                "VALUES (1000, 'call', '5551234', 0)",
        )
        db.version = 14
        db.close()
    }

    @Test
    fun `migration preserves outbox and contacts and adds cadence policies`() = runBlocking {
        createV14Database()

        val db = Room.databaseBuilder(context, AppDatabase::class.java, dbFile.absolutePath)
            .addMigrations(*REGISTERED_MIGRATIONS.toTypedArray())
            .build()

        // Neither of these tables is touched by this migration — they must survive untouched.
        val pending = db.pendingInteractionDao().getUnsynced()
        assertEquals(1, pending.size)
        assertEquals("5551234", pending[0].phoneNumber)
        val contact = db.cachedContactDao().getById(1)
        assertEquals("Dana White", contact?.fn)

        // The new table exists and is fully usable through the DAO, not just present as an
        // empty shell that happens to satisfy onValidateSchema.
        db.cachedCadencePolicyDao().upsert(
            CachedCadencePolicy(
                id = "policy-1",
                entityId = "contact-1",
                targetIntervalDays = 30,
                qualifyingTypes = listOf("call", "message"),
            ),
        )
        val policies = db.cachedCadencePolicyDao().getForContact("contact-1")
        assertEquals(1, policies.size)
        assertEquals(30, policies[0].targetIntervalDays)

        db.close()
    }
}
