package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.domain.repository.PendingInteraction
import com.mycorrhizal.crm.domain.repository.PendingInteractionRepository
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * ADR 0028 Decision 1 extracted the outbox→server mapping out of
 * `InteractionSyncWorker` (feature:tracking) into this core:data class so the
 * profile-switch drain reuses it. This pins the mapping's behavior in the
 * module that now owns it.
 */
class PendingInteractionFlusherTest {

    private val repository = mockk<PendingInteractionRepository>(relaxed = true)
    private val api = mockk<ApiClient>()
    private val flusher = PendingInteractionFlusher(repository, api)

    private fun interaction(
        id: Long = 1,
        kind: String = "call",
        direction: String? = "incoming",
        contactId: Int? = 7,
        key: String? = "key-1",
    ) = PendingInteraction(
        id = id,
        timestampMillis = 1_700_000_000_000,
        kind = kind,
        direction = direction,
        phoneNumber = "+15550100",
        matchedContactId = contactId,
        idempotencyKey = key,
    )

    @Test
    fun `an empty outbox drains immediately`() = runTest {
        coEvery { repository.unsynced() } returns emptyList()

        assertEquals(0, flusher.flush())
        assertTrue(flusher.drain())
        coVerify(exactly = 0) { api.createActivity(any(), any()) }
    }

    @Test
    fun `a call row syncs with its key and is marked`() = runTest {
        coEvery { repository.unsynced() } returnsMany listOf(listOf(interaction()), emptyList())
        coEvery {
            api.createActivity(
                match { it.type == "call" && it.title == "Call" && it.contactIds == listOf(7) && it.externalRef == "device:1" },
                "key-1",
            )
        } returns Result.success(Activity(id = 99))

        assertEquals(0, flusher.flush())
        coVerify { repository.markSynced(1, any()) }
        coVerify { repository.deleteSynced() }
    }

    @Test
    fun `a message row maps to a bodyless message activity`() = runTest {
        coEvery { repository.unsynced() } returnsMany listOf(listOf(interaction(kind = "message", direction = "incoming")), emptyList())
        coEvery {
            api.createActivity(
                match { it.type == "message" && it.title == "Message" && it.description == null },
                "key-1",
            )
        } returns Result.success(Activity(id = 5))

        flusher.flush()

        coVerify(exactly = 1) { api.createActivity(match { it.title == "Message" }, "key-1") }
    }

    @Test
    fun `a 404 on a linked contact drops the link and retries unassociated`() = runTest {
        coEvery { repository.unsynced() } returnsMany listOf(listOf(interaction()), emptyList())
        coEvery {
            api.createActivity(match { it.contactIds == listOf(7) }, "key-1")
        } returns Result.failure(ApiError.Client(404, "One or more contacts not found"))
        coEvery {
            api.createActivity(match { it.contactIds == null }, "key-1")
        } returns Result.success(Activity(id = 1))

        flusher.flush()

        coVerify { repository.clearMatchedContact(1) }
        coVerify { repository.markSynced(1, any()) }
    }

    @Test
    fun `a network failure leaves the row queued`() = runTest {
        coEvery { repository.unsynced() } returns listOf(interaction())
        coEvery { api.createActivity(any(), any()) } returns Result.failure(ApiError.Server(500, "boom"))

        assertEquals(1, flusher.flush())
        assertFalse(flusher.drain())
        coVerify(exactly = 0) { repository.markSynced(any(), any()) }
    }

    @Test
    fun `an unknown kind is skipped`() = runTest {
        coEvery { repository.unsynced() } returns listOf(interaction(kind = "weird"))

        flusher.flush()

        coVerify(exactly = 0) { api.createActivity(any(), any()) }
    }

    @Test
    fun `a keyless row is backfilled before syncing`() = runTest {
        coEvery { repository.unsynced() } returnsMany listOf(listOf(interaction(key = null)), emptyList())
        coEvery { repository.setIdempotencyKey(1, any()) } returns Unit
        coEvery { api.createActivity(any(), any()) } returns Result.success(Activity(id = 1))

        flusher.flush()

        coVerify { repository.setIdempotencyKey(1, any()) }
        coVerify { repository.markSynced(1, any()) }
    }
}
