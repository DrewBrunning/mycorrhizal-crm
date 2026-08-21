package com.mycorrhizal.crm.data.repository

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.data.local.AppDatabase
import com.mycorrhizal.crm.domain.repository.PendingInteraction
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class PendingInteractionRepositoryImplTest {

    private lateinit var db: AppDatabase
    private lateinit var repository: PendingInteractionRepositoryImpl

    @Before
    fun setup() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        db = Room.inMemoryDatabaseBuilder(context, AppDatabase::class.java).build()
        repository = PendingInteractionRepositoryImpl(db.pendingInteractionDao())
    }

    @After
    fun teardown() {
        db.close()
    }

    private fun interaction(timestampMillis: Long, synced: Boolean = false) = PendingInteraction(
        timestampMillis = timestampMillis,
        kind = "call",
        direction = "incoming",
        phoneNumber = "+15551234567",
        synced = synced,
    )

    @Test
    fun `record then unsynced returns the recorded interaction`() = runBlocking {
        repository.record(interaction(1000L))

        val unsynced = repository.unsynced()

        assertEquals(1, unsynced.size)
        assertEquals("+15551234567", unsynced.single().phoneNumber)
    }

    @Test
    fun `unsynced excludes already-synced rows`() = runBlocking {
        repository.record(interaction(1000L, synced = true))
        repository.record(interaction(2000L, synced = false))

        val unsynced = repository.unsynced()

        assertEquals(listOf(2000L), unsynced.map { it.timestampMillis })
    }

    @Test
    fun `markSynced flips a row out of the unsynced set`() = runBlocking {
        repository.record(interaction(1000L))
        val id = repository.unsynced().single().id

        repository.markSynced(id, "2026-08-21T00:00:00Z")

        assertTrue(repository.unsynced().isEmpty())
    }

    @Test
    fun `deleteSynced removes only synced rows`() = runBlocking {
        repository.record(interaction(1000L))
        val id = repository.unsynced().single().id
        repository.markSynced(id, "2026-08-21T00:00:00Z")
        repository.record(interaction(2000L))

        repository.deleteSynced()

        val remaining = repository.unsynced()
        assertEquals(1, remaining.size)
        assertEquals(2000L, remaining.single().timestampMillis)
    }
}
