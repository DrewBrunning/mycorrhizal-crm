package com.mycorrhizal.crm.data.local

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import java.io.File
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issue #385: [LocalDataCleaner] is what actually runs on logout / an
 * invalid-token account removal, so its two effects — wiping the Room mirror
 * and deleting the cached-image directory — are exercised directly against a
 * real (unencrypted, in-memory) [AppDatabase] and a real Robolectric
 * [Context.cacheDir], rather than only asserted on source text the way
 * [RoomEncryptionGuardTest] must for the SQLCipher-specific pieces.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class LocalDataCleanerTest {

    private lateinit var context: Context
    private lateinit var db: AppDatabase
    private lateinit var cleaner: LocalDataCleaner

    @Before
    fun setup() {
        context = ApplicationProvider.getApplicationContext()
        db = Room.inMemoryDatabaseBuilder(context, AppDatabase::class.java).build()
        cleaner = LocalDataCleaner(context, db)
    }

    @After
    fun teardown() {
        db.close()
    }

    @Test
    fun `clear wipes cached rows and deletes cached files`() = runBlocking {
        db.cachedContactDao().upsert(CachedContact(id = 1, fn = "Alice", firstname = "Alice"))
        val cachedFile = File(context.cacheDir, "leftover-photo.jpg").apply {
            parentFile?.mkdirs()
            writeText("stale profile photo bytes")
        }
        assertFalse("test setup sanity check", !cachedFile.exists())

        cleaner.clear()

        assertNull(
            "clear() must wipe the Room mirror (offline PII)",
            db.cachedContactDao().getById(1),
        )
        assertFalse(
            "clear() must delete the cached-image directory",
            cachedFile.exists(),
        )
    }

    @Test
    fun `clear is a no-op on an already-empty cache`() = runBlocking {
        // Nothing cached, no cacheDir contents — must not throw.
        cleaner.clear()

        assertNull(db.cachedContactDao().getById(1))
    }
}
