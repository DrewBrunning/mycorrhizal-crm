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
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * ANDROID-02 (issue #479): [MIGRATION_16_17] adds `pending_interactions.idempotencyKey` and
 * backfills it. Same rationale as its three predecessors for why this is hand-written rather
 * than left to `fallbackToDestructiveMigration` — the "before" database here carries an outbox
 * row through the hop, and losing the outbox on upgrade is the exact data-loss the registered
 * migration chain exists to prevent.
 *
 * The "before" shape is the exported v16 schema (issue #212's `isFavorite` present) built by
 * [LocalDatabaseSchemaFixtures.createV16Tables], whose `pending_interactions` still has no
 * `idempotencyKey` column — the shape a real v16 install upgrading to v17 has on disk.
 *
 * Issue #385 (SQLCipher): runs against the plain framework SQLite factory, same caveat as
 * [Migration13To14Test] — the encrypted real-device counterpart is
 * `RoomMigrationEncryptedTest` in `app/src/androidTest`.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class Migration16To17Test {

    private lateinit var context: Context
    private lateinit var dbFile: java.io.File

    @Before
    fun setup() {
        context = ApplicationProvider.getApplicationContext()
        dbFile = context.getDatabasePath("migration-16-17-test.db")
        dbFile.delete()
    }

    @After
    fun teardown() {
        dbFile.delete()
    }

    private fun createV16Database() {
        val db = SQLiteDatabase.openOrCreateDatabase(dbFile, null)
        LocalDatabaseSchemaFixtures.createV16Tables(db)
        db.execSQL(
            "INSERT INTO cached_contacts (id, fn, primaryPhone, phonesNormalized, archived, deleted, isFavorite) " +
                "VALUES (1, 'Dana White', '(800) 555-1234', '8005551234 5551234', 0, 0, 1)",
        )
        db.execSQL(
            "INSERT INTO pending_interactions (timestampMillis, kind, direction, phoneNumber, matchedContactId, synced, syncedAt) " +
                "VALUES (1000, 'call', 'incoming', '5551234', 1, 0, NULL)",
        )
        db.execSQL(
            "INSERT INTO pending_interactions (timestampMillis, kind, phoneNumber, synced) " +
                "VALUES (2000, 'message', '5559999', 0)",
        )
        db.execSQL(
            "INSERT INTO pending_interactions (timestampMillis, kind, phoneNumber, synced, syncedAt) " +
                "VALUES (3000, 'call', '5550000', 1, '2026-08-01T00:00:00Z')",
        )
        db.version = 16
        db.close()
    }

    @Test
    fun `v16 outbox survives the hop and every row gains a backfilled idempotency key`() = runBlocking {
        createV16Database()

        val db = Room.databaseBuilder(context, AppDatabase::class.java, dbFile.absolutePath)
            .addMigrations(*REGISTERED_MIGRATIONS.toTypedArray())
            .build()

        // The pre-existing rows survived (this migration's whole reason to exist), still
        // unsynced, still matched.
        val pending = db.pendingInteractionDao().getUnsynced()
        assertEquals(2, pending.size)
        assertEquals("5551234", pending.single { it.kind == "call" }.phoneNumber)
        assertEquals(1, pending.single { it.kind == "call" }.matchedContactId)

        // ANDROID-02: every pre-existing row — including rows an old build queued while the
        // device was offline — has a retry key after the hop, so a v17 worker can sync them
        // retry-safely with no special-casing. The backfill is `lower(hex(randomblob(16)))`:
        // a 32-char lowercase hex string.
        for (row in pending) {
            assertNotNull("every migrated outbox row must have an idempotencyKey", row.idempotencyKey)
            assertTrue(
                "idempotencyKey must be backfilled as 32-char hex, was '${row.idempotencyKey}'",
                Regex("[0-9a-f]{32}").matches(row.idempotencyKey.orEmpty()),
            )
        }

        // The synced (previously-cleaned-but-not-yet-deleted) row gained a key too — raw-count,
        // since getUnsynced() excludes it by design.
        val allRows = withContext(Dispatchers.IO) {
            db.query("SELECT idempotencyKey FROM pending_interactions WHERE synced = 1", null)
        }
        allRows.use { cursor ->
            assertTrue(cursor.moveToFirst())
            val key = cursor.getString(cursor.getColumnIndexOrThrow("idempotencyKey"))
            assertTrue("synced row must be backfilled too, was '$key'", Regex("[0-9a-f]{32}").matches(key))
        }

        // The v16-era isFavorite column is untouched by this hop.
        assertEquals(true, db.cachedContactDao().getById(1)?.isFavorite)

        // Backfilled keys are writable through the DAO, not just present (the worker's
        // defensive setIdempotencyKey path must actually work post-migration).
        val target = pending[0].id
        db.pendingInteractionDao().setIdempotencyKey(target, "deadbeef-dead-beef-dead-beefdeadbeef")
        val updated = db.pendingInteractionDao().getUnsynced().single { it.id == target }
        assertEquals("deadbeef-dead-beef-dead-beefdeadbeef", updated.idempotencyKey)
        assertEquals(2, db.pendingInteractionDao().countUnsynced())

        db.close()
    }
}
