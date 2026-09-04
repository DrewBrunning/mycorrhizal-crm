package com.mycorrhizal.crm.data.local

import android.content.Context
import android.database.sqlite.SQLiteDatabase
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issue #212 (web #173): [MIGRATION_15_16] adds `cached_contacts.isFavorite`, a single
 * `ALTER TABLE ADD COLUMN` with a `NOT NULL DEFAULT 0`. Same rationale as [Migration13To14Test]
 * for why this is hand-written rather than left to `fallbackToDestructiveMigration` — the
 * "before" database here carries an outbox row and a cadence policy row through the hop too,
 * even though this migration's own SQL only touches `cached_contacts`.
 *
 * Issue #385 (SQLCipher): runs against the plain framework SQLite factory, same caveat as
 * [Migration13To14Test] — see that class's doc comment.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class Migration15To16Test {

    private lateinit var context: Context
    private lateinit var dbFile: java.io.File

    @Before
    fun setup() {
        context = ApplicationProvider.getApplicationContext()
        dbFile = context.getDatabasePath("migration-15-16-test.db")
        dbFile.delete()
    }

    @After
    fun teardown() {
        dbFile.delete()
    }

    private fun createV15Database() {
        val db = SQLiteDatabase.openOrCreateDatabase(dbFile, null)
        LocalDatabaseSchemaFixtures.createV15Tables(db)
        db.execSQL(
            "INSERT INTO cached_contacts (id, fn, primaryPhone, phonesNormalized, archived, deleted) " +
                "VALUES (1, 'Dana White', '(800) 555-1234', '8005551234 5551234', 0, 0)",
        )
        db.execSQL(
            "INSERT INTO pending_interactions (timestampMillis, kind, phoneNumber, synced) " +
                "VALUES (1000, 'call', '5551234', 0)",
        )
        db.execSQL(
            "INSERT INTO cached_cadence_policies " +
                "(id, entityId, targetIntervalDays, qualifyingTypes, deleted) " +
                "VALUES ('policy-1', 'contact-1', 30, '[\"call\",\"message\"]', 0)",
        )
        db.version = 15
        db.close()
    }

    @Test
    fun `migration preserves outbox and cadence policies and adds isFavorite defaulted false`() = runBlocking {
        createV15Database()

        val db = Room.databaseBuilder(context, AppDatabase::class.java, dbFile.absolutePath)
            .addMigrations(*REGISTERED_MIGRATIONS.toTypedArray())
            .build()

        // Neither of these tables is touched by this migration — they must survive untouched.
        val pending = db.pendingInteractionDao().getUnsynced()
        assertEquals(1, pending.size)
        assertEquals("5551234", pending[0].phoneNumber)
        val policies = db.cachedCadencePolicyDao().getForContact("contact-1")
        assertEquals(1, policies.size)

        // The pre-existing contact row survived, and its new column defaulted to false rather
        // than silently favoriting every pre-existing contact.
        val contact = db.cachedContactDao().getById(1)
        assertEquals("Dana White", contact?.fn)
        assertFalse(contact?.isFavorite ?: true)

        // The column is fully usable through the DAO, not just present as a schema-only shell.
        db.cachedContactDao().setFavorite(1, true)
        assertEquals(true, db.cachedContactDao().getById(1)?.isFavorite)

        db.close()
    }
}
