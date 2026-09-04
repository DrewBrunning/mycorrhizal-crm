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
 * T76's real hazard: `pending_interactions` is a not-yet-synced outbox
 * (`AppDatabase`'s doc comment), not rebuildable cache data — a destructive migration on this
 * version bump would silently delete it, which is why [MIGRATION_13_14] is hand-written instead
 * of relying on `fallbackToDestructiveMigration`. This test builds a real v13-shaped database
 * (the pre-T76 schema, captured from a clean checkout's generated Room code), migrates it with
 * the real database builder, and asserts both halves of the ticket: no data loss, and the new
 * phone index actually works.
 *
 * Issue #385 (SQLCipher): this test deliberately runs against the **plain** framework SQLite
 * factory, not production's encrypted `SupportOpenHelperFactory`. SQLCipher's `libsqlcipher.so`
 * is an Android-native binary (links Bionic) and cannot be loaded in the JVM/Robolectric, so the
 * migration *SQL* semantics are what this test pins; the encrypted-factory equivalent — the same
 * migration chain plus the plaintext→encrypted transition and FTS search under encryption — is
 * covered by the instrumented `RoomCacheEncryptionTest` (`app/src/androidTest`).
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class Migration13To14Test {

    private lateinit var context: Context
    private lateinit var dbFile: java.io.File

    @Before
    fun setup() {
        context = ApplicationProvider.getApplicationContext()
        dbFile = context.getDatabasePath("migration-13-14-test.db")
        dbFile.delete()
    }

    @After
    fun teardown() {
        dbFile.delete()
    }

    /**
     * Hand-builds the exact v13 schema via [LocalDatabaseSchemaFixtures] — every table Room's
     * `RoomOpenDelegate.onValidateSchema` checks after a migration, not just the two this
     * migration touches, or validation fails on tables that were never given a chance to differ.
     */
    private fun createV13Database() {
        val db = SQLiteDatabase.openOrCreateDatabase(dbFile, null)
        LocalDatabaseSchemaFixtures.createV13Tables(db)
        db.execSQL(
            "INSERT INTO cached_contacts (id, fn, primaryPhone, archived, deleted) " +
                "VALUES (1, 'Dana White', '(800) 555-1234', 0, 0)",
        )
        db.execSQL(
            "INSERT INTO pending_interactions (timestampMillis, kind, phoneNumber, synced) " +
                "VALUES (1000, 'call', '5551234', 0)",
        )
        db.version = 13
        db.close()
    }

    @Test
    fun `migration preserves the pending outbox and enables phone search`() = runBlocking {
        createV13Database()

        val db = Room.databaseBuilder(context, AppDatabase::class.java, dbFile.absolutePath)
            // 13->16: the full REGISTERED_MIGRATIONS chain is registered so 13->14->15->16
            // reaches the current version — this test's subject stays the 13->14 hop (outbox +
            // FTS columns); [Migration14To15Test]/[Migration15To16Test] cover the other two hops.
            .addMigrations(*REGISTERED_MIGRATIONS.toTypedArray())
            .build()

        // The not-yet-synced outbox row survived — the whole reason this migration is
        // hand-written instead of destructive.
        val pending = db.pendingInteractionDao().getUnsynced()
        assertEquals(1, pending.size)
        assertEquals("5551234", pending[0].phoneNumber)

        // The pre-existing cached_contacts row is still there too (an ALTER TABLE ADD COLUMN
        // preserves rows; only a destructive rebuild would have dropped it).
        val contact = db.cachedContactDao().getById(1)
        assertEquals("Dana White", contact?.fn)

        // The FTS mirror was dropped, recreated with the new column, and its content-sync
        // triggers rebuilt by Room's own migration pipeline (onPostMigrate) — proven by
        // writing a fresh row through the DAO (not raw SQL) and confirming the recreated
        // triggers actually keep cached_contacts_fts in sync under the new column set.
        db.cachedContactDao().upsert(
            CachedContact(
                id = 2,
                fn = "New Contact",
                primaryPhone = "555-0100",
                phonesNormalized = PhoneKey.flatten(listOf("555-0100")),
            ),
        )
        val found = db.cachedContactDao().searchFtsMatch("phonesNormalized:5550100*")
        assertEquals(1, found.size)
        assertEquals("New Contact", found[0].fn)

        db.close()
    }
}
