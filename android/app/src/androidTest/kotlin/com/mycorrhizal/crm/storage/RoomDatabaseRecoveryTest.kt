package com.mycorrhizal.crm.storage

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.mycorrhizal.crm.data.local.AppDatabase
import com.mycorrhizal.crm.data.local.CachedContact
import com.mycorrhizal.crm.data.local.RoomDatabaseRecovery
import java.io.File
import kotlinx.coroutines.runBlocking
import net.zetetic.database.sqlcipher.SupportOpenHelperFactory
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Issue #998: an encrypted Room mirror that was left keyed with a passphrase
 * this process no longer has (the `RoomPassphraseStore` async-persist race
 * that motivated this ticket, or any other corruption) must recover instead
 * of boot-looping. Instrumented because SQLCipher's native library — and the
 * "wrong key fails on first real query, not on build()" behavior this
 * recovery has to work around (see [RoomCacheEncryptionTest]) — only runs
 * here, not in Robolectric.
 */
@RunWith(AndroidJUnit4::class)
class RoomDatabaseRecoveryTest {

    private val context: Context = ApplicationProvider.getApplicationContext()

    private lateinit var dbFile: File

    @Before
    fun setUp() {
        System.loadLibrary("sqlcipher")
        dbFile = context.getDatabasePath("room-recovery-test.db")
        dbFile.delete()
        deleteSidecars(dbFile)
    }

    @After
    fun tearDown() {
        dbFile.delete()
        deleteSidecars(dbFile)
    }

    private fun deleteSidecars(file: File) {
        listOf("-journal", "-wal", "-shm").forEach { suffix ->
            File(file.parentFile, file.name + suffix).delete()
        }
    }

    private fun builder(key: String): () -> AppDatabase = {
        Room.databaseBuilder(context, AppDatabase::class.java, dbFile.absolutePath)
            .openHelperFactory(SupportOpenHelperFactory(key.toByteArray()))
            .fallbackToDestructiveMigration()
            .build()
    }

    @Test
    fun opensNormallyAndKeepsExistingDataWhenThePassphraseIsCorrect() {
        val key = "a".repeat(64)
        val seeded = RoomDatabaseRecovery.openOrRebuild(dbFile, builder(key))
        runBlocking {
            seeded.cachedContactDao().upsert(CachedContact(id = 1, fn = "Dana White"))
        }
        seeded.close()

        val reopened = RoomDatabaseRecovery.openOrRebuild(dbFile, builder(key))
        runBlocking {
            assertEquals("Dana White", reopened.cachedContactDao().getById(1)?.fn)
        }
        reopened.close()
    }

    @Test
    fun wipesAndRebuildsWhenTheDatabaseIsKeyedWithAnUnrecoverablyDifferentPassphrase() {
        val lostKey = "b".repeat(64)
        val seeded = RoomDatabaseRecovery.openOrRebuild(dbFile, builder(lostKey))
        runBlocking {
            seeded.cachedContactDao().upsert(CachedContact(id = 1, fn = "Dana White"))
        }
        seeded.close()

        // Simulate the next launch generating a different passphrase because
        // the one that actually encrypted the file above was never
        // recoverable (e.g. lost to the pre-#998 async-persist race).
        val newKey = "c".repeat(64)
        val recovered = RoomDatabaseRecovery.openOrRebuild(dbFile, builder(newKey))
        try {
            // The recovery must produce a database that opens and is usable
            // under the new key...
            runBlocking {
                assertNull("The wiped mirror must not carry over the old (now-unreadable) data", recovered.cachedContactDao().getById(1))
                recovered.cachedContactDao().upsert(CachedContact(id = 2, fn = "Alex Rivera"))
                assertEquals("Alex Rivera", recovered.cachedContactDao().getById(2)?.fn)
            }
        } finally {
            recovered.close()
        }

        // ...and the rebuild must actually be durable on disk, not just an
        // in-memory recovery: reopening under the new key again must still work.
        val reopened = RoomDatabaseRecovery.openOrRebuild(dbFile, builder(newKey))
        runBlocking {
            assertEquals("Alex Rivera", reopened.cachedContactDao().getById(2)?.fn)
        }
        reopened.close()
    }
}
