package com.mycorrhizal.crm.data.repository

import android.content.Context
import androidx.room.Room
import androidx.room.withTransaction
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.data.local.AppDatabase
import com.mycorrhizal.crm.domain.repository.OUTBOX_UNSYNCED_CAP
import com.mycorrhizal.crm.domain.repository.PendingInteraction
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestCoroutineScheduler
import kotlinx.coroutines.test.runTest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
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

    // Room's DAO suspend functions hop to Room's query context with withContext. On Room's
    // default background executor that hop races the caller: when the query finishes before
    // the caller checks for suspension the call returns inline, otherwise it suspends. The
    // repository's Unit-returning wrappers (markSynced, deleteSynced, setIdempotencyKey,
    // clearMatchedContact) have a separate bytecode path for each outcome, so which ones ran
    // varied per run and the file's JaCoCo line coverage flapped across the Android coverage
    // ratchet's tolerance. Pinning Room to its own test dispatcher on the test's scheduler
    // makes every DAO hop a real, deterministic suspension (the queued query cannot run until
    // the test body has suspended); the in-transaction test below covers the inline path.
    private val scheduler = TestCoroutineScheduler()
    private val roomDispatcher = StandardTestDispatcher(scheduler, name = "room")

    private lateinit var db: AppDatabase
    private lateinit var repository: PendingInteractionRepositoryImpl

    private fun runRepoTest(body: suspend () -> Unit) =
        runTest(StandardTestDispatcher(scheduler)) { body() }

    @Before
    fun setup() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        db = Room.inMemoryDatabaseBuilder(context, AppDatabase::class.java)
            .setQueryCoroutineContext(roomDispatcher)
            // The test dispatcher runs on Robolectric's main (test) thread.
            .allowMainThreadQueries()
            .build()
        repository = PendingInteractionRepositoryImpl(db.pendingInteractionDao(), db)
    }

    @After
    fun teardown() {
        db.close()
    }

    private fun interaction(
        timestampMillis: Long,
        synced: Boolean = false,
        idempotencyKey: String? = null
    ) = PendingInteraction(
        timestampMillis = timestampMillis,
        kind = "call",
        direction = "incoming",
        phoneNumber = "+15551234567",
        synced = synced,
        idempotencyKey = idempotencyKey
    )

    @Test
    fun `record then unsynced returns the recorded interaction`() = runRepoTest {
        repository.record(interaction(1000L))

        val unsynced = repository.unsynced()

        assertEquals(1, unsynced.size)
        assertEquals("+15551234567", unsynced.single().phoneNumber)
    }

    @Test
    fun `unsynced excludes already-synced rows`() = runRepoTest {
        repository.record(interaction(1000L, synced = true))
        repository.record(interaction(2000L, synced = false))

        val unsynced = repository.unsynced()

        assertEquals(listOf(2000L), unsynced.map { it.timestampMillis })
    }

    @Test
    fun `markSynced flips a row out of the unsynced set`() = runRepoTest {
        repository.record(interaction(1000L))
        val id = repository.unsynced().single().id

        repository.markSynced(id, "2026-08-21T00:00:00Z")

        assertTrue(repository.unsynced().isEmpty())
    }

    @Test
    fun `deleteSynced removes only synced rows`() = runRepoTest {
        repository.record(interaction(1000L))
        val id = repository.unsynced().single().id
        repository.markSynced(id, "2026-08-21T00:00:00Z")
        repository.record(interaction(2000L))

        repository.deleteSynced()

        val remaining = repository.unsynced()
        assertEquals(1, remaining.size)
        assertEquals(2000L, remaining.single().timestampMillis)
    }

    // --- ANDROID-02 (issue #479): idempotency keys + bounded queue ----------

    @Test
    fun `record assigns a fresh idempotency key when the caller supplies none`() = runRepoTest {
        repository.record(interaction(1000L))

        val recorded = repository.unsynced().single()

        assertNotNull("record() must assign an idempotencyKey", recorded.idempotencyKey)
        assertEquals("assigned key must be a UUID", 36, recorded.idempotencyKey!!.length)
    }

    @Test
    fun `record preserves a caller-supplied idempotency key`() = runRepoTest {
        repository.record(interaction(1000L, idempotencyKey = "caller-key"))

        assertEquals("caller-key", repository.unsynced().single().idempotencyKey)
    }

    @Test
    fun `distinct rows get distinct idempotency keys`() = runRepoTest {
        repository.record(interaction(1000L))
        repository.record(interaction(2000L))

        val keys = repository.unsynced().map { it.idempotencyKey }
        assertEquals(2, keys.distinct().size)
    }

    @Test
    fun `setIdempotencyKey persists a key onto an existing row`() = runRepoTest {
        repository.record(interaction(1000L))
        val id = repository.unsynced().single().id

        repository.setIdempotencyKey(id, "replacement-key")

        assertEquals("replacement-key", repository.unsynced().single().idempotencyKey)
    }

    @Test
    fun `clearMatchedContact drops the contact link but keeps the row`() = runRepoTest {
        repository.record(
            interaction(1000L).copy(matchedContactId = 7)
        )
        val id = repository.unsynced().single().id
        assertEquals(7, repository.unsynced().single().matchedContactId)

        repository.clearMatchedContact(id)

        val after = repository.unsynced().single()
        assertEquals(null, after.matchedContactId)
        assertEquals("+15551234567", after.phoneNumber)
        assertNotNull("clearing the link must not clear the retry key", after.idempotencyKey)
    }

    @Test
    fun `the unsynced queue never exceeds the cap - oldest rows are evicted`() = runRepoTest {
        // Insert one more than the cap. Every row is a distinct logical interaction; the
        // bound (ANDROID-02 recommended action 7) must hold no matter how long a device is
        // offline, keeping the NEWEST entries.
        val overflow = OUTBOX_UNSYNCED_CAP + 5
        for (i in 0 until overflow) {
            repository.record(interaction(1000L + i))
        }

        assertEquals(OUTBOX_UNSYNCED_CAP, db.pendingInteractionDao().countUnsynced())
        val kept = repository.unsynced().map { it.timestampMillis }
        assertEquals("the newest entries must survive eviction", 1000L + overflow - 1, kept.max())
        assertEquals("the oldest 5 must be evicted", 1000L + 5, kept.min())
    }

    @Test
    fun `the cap only applies to unsynced rows`() = runRepoTest {
        // A synced row awaiting deleteSynced() cleanup must never be evicted by the unsynced
        // bound (it is the caller's record of what already synced; deleteSynced owns it).
        repository.record(interaction(1000L))
        val id = repository.unsynced().single().id
        repository.markSynced(id, "2026-08-21T00:00:00Z")
        for (i in 0 until OUTBOX_UNSYNCED_CAP) {
            repository.record(interaction(1000L + i + 1))
        }

        assertEquals(OUTBOX_UNSYNCED_CAP, db.pendingInteractionDao().countUnsynced())
        val totalRows = run {
            db.query("SELECT COUNT(*) AS c FROM pending_interactions", null).use { c ->
                c.moveToFirst()
                c.getInt(c.getColumnIndexOrThrow("c"))
            }
        }
        assertEquals(
            "the synced row must remain alongside the capped unsynced set",
            OUTBOX_UNSYNCED_CAP + 1,
            totalRows
        )
    }

    @Test
    fun `keys survive markSynced and are readable until deleteSynced`() = runRepoTest {
        repository.record(interaction(1000L))
        val row = repository.unsynced().single()
        val key = row.idempotencyKey

        repository.markSynced(row.id, "2026-08-21T00:00:00Z")
        // markSynced leaves the row present (synced=1) for deleteSynced cleanup; re-query it by
        // its raw existence to prove the key wasn't lost in the flip.
        val all = run {
            db.query("SELECT idempotencyKey FROM pending_interactions WHERE id = ${row.id}", null)
        }
        all.use { cursor ->
            assertTrue(cursor.moveToFirst())
            assertEquals(key, cursor.getString(cursor.getColumnIndexOrThrow("idempotencyKey")))
        }
    }

    @Test
    fun `writes nested in a caller's transaction complete without the DAO suspending`() =
        runRepoTest {
            // Inside an open transaction Room runs DAO calls inline on the transaction's
            // context, so each call returns its result without suspending. Every other test
            // here reaches the DAO through a real suspension (see roomDispatcher), so this is
            // the test that deterministically exercises the repository's no-suspension path.
            db.withTransaction {
                repository.record(interaction(1000L).copy(matchedContactId = 7))
            }
            val id = repository.unsynced().single().id

            db.withTransaction {
                repository.setIdempotencyKey(id, "in-tx-key")
                repository.clearMatchedContact(id)
            }
            val row = repository.unsynced().single()
            assertEquals("in-tx-key", row.idempotencyKey)
            assertNull(row.matchedContactId)

            db.withTransaction {
                repository.markSynced(id, "2026-08-21T00:00:00Z")
                repository.deleteSynced()
            }
            val totalRows = db.query("SELECT COUNT(*) FROM pending_interactions", null).use { c ->
                c.moveToFirst()
                c.getInt(0)
            }
            assertEquals("deleteSynced must remove the row markSynced flipped", 0, totalRows)
        }

    // --- Issue #721: recordIfNew exact-match dedupe --------------------------

    @Test
    fun `recordIfNew stages a brand-new interaction and returns true`() = runRepoTest {
        val inserted = repository.recordIfNew(interaction(1000L))

        assertTrue(inserted)
        assertEquals(1, repository.unsynced().size)
    }

    @Test
    fun `recordIfNew skips an identical already-staged interaction and returns false`() =
        runRepoTest {
            repository.record(interaction(1000L))

            // Same kind / phone / timestamp as the existing row — the signature an
            // overlapping reader run yields for the same underlying message/call.
            val inserted = repository.recordIfNew(interaction(1000L))

            assertEquals(false, inserted)
            assertEquals(1, repository.unsynced().size)
        }

    @Test
    fun `recordIfNew dedupe is exact - a different timestamp stages a new row`() = runRepoTest {
        repository.record(interaction(1000L))

        val inserted = repository.recordIfNew(interaction(2000L))

        assertTrue(inserted)
        assertEquals(listOf(1000L, 2000L), repository.unsynced().map { it.timestampMillis })
    }

    @Test
    fun `recordIfNew dedupe distinguishes by phone number and kind`() = runRepoTest {
        repository.record(interaction(1000L))

        val differentPhone = repository.recordIfNew(
            interaction(1000L).copy(phoneNumber = "+15559999999")
        )
        val differentKind = repository.recordIfNew(
            interaction(1000L).copy(kind = "message")
        )

        assertTrue(differentPhone)
        assertTrue(differentKind)
        assertEquals(3, repository.unsynced().size)
    }

    @Test
    fun `recordIfNew still applies the bounded-queue cap`() = runRepoTest {
        for (i in 0 until OUTBOX_UNSYNCED_CAP) {
            repository.record(interaction(1000L + i))
        }
        // One over the cap through the dedupe path.
        repository.recordIfNew(interaction(1000L + OUTBOX_UNSYNCED_CAP))

        assertEquals(OUTBOX_UNSYNCED_CAP, db.pendingInteractionDao().countUnsynced())
    }
}
