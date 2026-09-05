package com.mycorrhizal.crm.feature.tracking

import com.mycorrhizal.crm.domain.repository.PendingInteraction
import com.mycorrhizal.crm.domain.repository.PendingInteractionRepository
import com.mycorrhizal.crm.domain.repository.TrackingSettingsRepository
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
class SmsReaderTest {

    @Test
    fun `parseFromExtras returns null without messages`() {
        val intent = android.content.Intent()
        assertEquals(null, SmsReader.parseFromExtras(intent))
    }
}

/**
 * The outbox→server mapping (ANDROID-02/issue #479): rows sync as Activities
 * with an idempotency key on every attempt, an unknown kind is skipped, a row
 * whose matched contact vanished server-side (404) is re-synced unassociated
 * instead of stuck, and a plain network failure leaves the row for the next
 * run. The worker is constructed directly (its @AssistedInject shape) with
 * mocked repositories, mirroring the sibling worker tests.
 */
class InteractionSyncWorkerLogicTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val pendingRepo = mockk<PendingInteractionRepository>()
    private val trackingSettings = mockk<TrackingSettingsRepository>()
    private val apiClient = mockk<ApiClient>()

    private fun worker() = InteractionSyncWorker(
        appContext = mockk(relaxed = true),
        workerParams = mockk(relaxed = true),
        pendingInteractionRepository = pendingRepo,
        trackingSettings = trackingSettings,
        apiClient = apiClient,
    )

    private fun enabledTracking() {
        coEvery { trackingSettings.callTrackingEnabled() } returns true
        coEvery { trackingSettings.smsTrackingEnabled() } returns true
    }

    @Test
    fun `call interaction maps to a call activity with the matched contact`() = runTest {
        enabledTracking()
        coEvery { pendingRepo.unsynced() } returns listOf(
            PendingInteraction(
                id = 1,
                timestampMillis = 1_700_000_000_000,
                kind = InteractionCapture.KIND_CALL,
                direction = InteractionCapture.DIR_INCOMING,
                phoneNumber = "+15550100",
                matchedContactId = 7,
                idempotencyKey = "key-1",
            ),
        )
        coEvery {
            apiClient.createActivity(
                match { it.type == "call" && it.title == "Call" && it.contactIds == listOf(7) },
                "key-1",
            )
        } returns Result.success(com.mycorrhizal.crm.model.network.Activity(id = 99))
        coEvery { pendingRepo.markSynced(1, any()) } returns Unit
        coEvery { pendingRepo.deleteSynced() } returns Unit

        val result = worker().doWork()

        assertEquals(androidx.work.ListenableWorker.Result.success(), result)
        coVerify { pendingRepo.markSynced(1, any()) }
    }

    @Test
    fun `missed call interaction maps to a missed-call title`() = runTest {
        enabledTracking()
        coEvery { pendingRepo.unsynced() } returns listOf(
            PendingInteraction(
                id = 2,
                timestampMillis = 1_700_000_000_000,
                kind = InteractionCapture.KIND_CALL,
                direction = InteractionCapture.DIR_MISSED,
                phoneNumber = "+15550100",
                idempotencyKey = "key-2",
            ),
        )
        coEvery {
            apiClient.createActivity(
                match { it.type == "call" && it.title == "Missed call" },
                "key-2",
            )
        } returns Result.success(com.mycorrhizal.crm.model.network.Activity(id = 100))
        coEvery { pendingRepo.markSynced(2, any()) } returns Unit
        coEvery { pendingRepo.deleteSynced() } returns Unit

        worker().doWork()
        coVerify { pendingRepo.markSynced(2, any()) }
    }

    @Test
    fun `message interaction maps to a message activity with no description`() = runTest {
        enabledTracking()
        coEvery { pendingRepo.unsynced() } returns listOf(
            PendingInteraction(
                id = 3,
                timestampMillis = 1_700_000_000_000,
                kind = InteractionCapture.KIND_MESSAGE,
                direction = InteractionCapture.DIR_INCOMING,
                phoneNumber = "+15550100",
                matchedContactId = 9,
                idempotencyKey = "key-3",
            ),
        )
        coEvery {
            apiClient.createActivity(
                match {
                    it.type == "message" &&
                        it.title == "Message" &&
                        it.contactIds == listOf(9) &&
                        it.description == null // privacy: body never sent
                },
                "key-3",
            )
        } returns Result.success(com.mycorrhizal.crm.model.network.Activity(id = 101))
        coEvery { pendingRepo.markSynced(3, any()) } returns Unit
        coEvery { pendingRepo.deleteSynced() } returns Unit

        worker().doWork()
        coVerify { pendingRepo.markSynced(3, any()) }
    }

    @Test
    fun `the row's idempotency key is sent as the request's retry key`() = runTest {
        enabledTracking()
        coEvery { pendingRepo.unsynced() } returns listOf(
            PendingInteraction(
                id = 4,
                timestampMillis = 1_700_000_000_000,
                kind = InteractionCapture.KIND_CALL,
                matchedContactId = 7,
                idempotencyKey = "row-uuid-4",
            ),
        )
        // The key on the wire must be the row's stored key — exactly-once retry depends on a
        // stable per-row key (ANDROID-02/issue #479, CON-04/ADR-0010).
        coEvery {
            apiClient.createActivity(match { it.externalRef == "device:4" }, "row-uuid-4")
        } returns Result.success(com.mycorrhizal.crm.model.network.Activity(id = 200))
        coEvery { pendingRepo.markSynced(4, any()) } returns Unit
        coEvery { pendingRepo.deleteSynced() } returns Unit

        worker().doWork()

        coVerify(exactly = 1) {
            apiClient.createActivity(match { it.externalRef == "device:4" }, "row-uuid-4")
        }
        coVerify { pendingRepo.markSynced(4, any()) }
    }

    @Test
    fun `a keyless row is backfilled with a persisted key before syncing`() = runTest {
        enabledTracking()
        coEvery { pendingRepo.unsynced() } returns listOf(
            PendingInteraction(
                id = 5,
                timestampMillis = 1_700_000_000_000,
                kind = InteractionCapture.KIND_CALL,
                matchedContactId = 7,
                idempotencyKey = null, // pre-v17 edge the migration backfill should prevent
            ),
        )
        coEvery { pendingRepo.setIdempotencyKey(5, any()) } returns Unit
        // Whatever key was generated + persisted must be the one sent.
        coEvery {
            apiClient.createActivity(
                match { it.contactIds == listOf(7) },
                match { it.length == 36 }, // a UUID
            )
        } returns Result.success(com.mycorrhizal.crm.model.network.Activity(id = 201))
        coEvery { pendingRepo.markSynced(5, any()) } returns Unit
        coEvery { pendingRepo.deleteSynced() } returns Unit

        worker().doWork()

        coVerify(exactly = 1) { pendingRepo.setIdempotencyKey(5, any()) }
        coVerify { pendingRepo.markSynced(5, any()) }
    }

    @Test
    fun `a row whose matched contact vanished server side is re-synced unassociated`() = runTest {
        enabledTracking()
        coEvery { pendingRepo.unsynced() } returns listOf(
            PendingInteraction(
                id = 6,
                timestampMillis = 1_700_000_000_000,
                kind = InteractionCapture.KIND_CALL,
                direction = InteractionCapture.DIR_INCOMING,
                phoneNumber = "+15550100",
                matchedContactId = 42,
                idempotencyKey = "key-6",
            ),
        )
        // First attempt references the now-deleted contact → 404 (ADR-0009: each queued write
        // resolves on its own; soft-deleted contacts are excluded from the create's lookup).
        coEvery {
            apiClient.createActivity(match { it.contactIds == listOf(42) }, "key-6")
        } returns Result.failure(ApiError.Client(404, "One or more contacts not found"))
        // After the link is dropped the row syncs unassociated — like an unmatched number.
        coEvery {
            apiClient.createActivity(match { it.contactIds == null }, "key-6")
        } returns Result.success(com.mycorrhizal.crm.model.network.Activity(id = 202))
        coEvery { pendingRepo.clearMatchedContact(6) } returns Unit
        coEvery { pendingRepo.markSynced(6, any()) } returns Unit
        coEvery { pendingRepo.deleteSynced() } returns Unit

        val result = worker().doWork()

        assertEquals(androidx.work.ListenableWorker.Result.success(), result)
        coVerify(exactly = 1) { pendingRepo.clearMatchedContact(6) }
        coVerify { pendingRepo.markSynced(6, any()) }
        // The interaction was recorded (never lost), just without the stale link.
    }

    @Test
    fun `a network failure does not drop the contact link and leaves the row unsynced`() = runTest {
        enabledTracking()
        coEvery { pendingRepo.unsynced() } returns listOf(
            PendingInteraction(
                id = 7,
                timestampMillis = 1_700_000_000_000,
                kind = InteractionCapture.KIND_CALL,
                matchedContactId = 42,
                idempotencyKey = "key-7",
            ),
        )
        coEvery { apiClient.createActivity(any(), "key-7") } returns
            Result.failure(ApiError.Network(java.io.IOException("offline")))
        coEvery { pendingRepo.deleteSynced() } returns Unit

        worker().doWork()

        coVerify(exactly = 0) { pendingRepo.clearMatchedContact(7) }
        coVerify(exactly = 0) { pendingRepo.markSynced(7, any()) }
    }

    @Test
    fun `a non-404 failure never drops the contact link`() = runTest {
        enabledTracking()
        coEvery { pendingRepo.unsynced() } returns listOf(
            PendingInteraction(
                id = 8,
                timestampMillis = 1_700_000_000_000,
                kind = InteractionCapture.KIND_CALL,
                matchedContactId = 42,
                idempotencyKey = "key-8",
            ),
        )
        coEvery { apiClient.createActivity(any(), "key-8") } returns
            Result.failure(ApiError.Client(400, "bad request"))
        coEvery { pendingRepo.deleteSynced() } returns Unit

        worker().doWork()

        coVerify(exactly = 0) { pendingRepo.clearMatchedContact(8) }
        coVerify(exactly = 0) { pendingRepo.markSynced(8, any()) }
    }

    @Test
    fun `an unknown interaction kind is skipped without a server call`() = runTest {
        enabledTracking()
        coEvery { pendingRepo.unsynced() } returns listOf(
            PendingInteraction(
                id = 9,
                timestampMillis = 1_700_000_000_000,
                kind = "unknown-kind",
                idempotencyKey = "key-9",
            ),
        )
        coEvery { pendingRepo.deleteSynced() } returns Unit

        worker().doWork()

        coVerify(exactly = 0) { apiClient.createActivity(any(), any()) }
        coVerify(exactly = 0) { pendingRepo.markSynced(9, any()) }
    }

    @Test
    fun `deleteSynced runs even when one upload in the batch fails`() = runTest {
        // Two pending interactions; only the first upload succeeds. Previously
        // deleteSynced() was gated on syncedCount == pending.size, so a single
        // failure left the successfully-synced row undeleted forever (it is
        // marked synced=1 and excluded from future unsynced() batches, so it
        // would only ever be cleaned up by a later fully-successful run).
        enabledTracking()
        coEvery { pendingRepo.unsynced() } returns listOf(
            PendingInteraction(
                id = 10,
                timestampMillis = 1_700_000_000_000,
                kind = InteractionCapture.KIND_CALL,
                direction = InteractionCapture.DIR_INCOMING,
                phoneNumber = "+15550100",
                idempotencyKey = "key-10",
            ),
            PendingInteraction(
                id = 11,
                timestampMillis = 1_700_000_001_000,
                kind = InteractionCapture.KIND_CALL,
                direction = InteractionCapture.DIR_INCOMING,
                phoneNumber = "+15550101",
                idempotencyKey = "key-11",
            ),
        )
        coEvery {
            apiClient.createActivity(match { it.externalRef == "device:10" }, "key-10")
        } returns Result.success(com.mycorrhizal.crm.model.network.Activity(id = 200))
        coEvery {
            apiClient.createActivity(match { it.externalRef == "device:11" }, "key-11")
        } returns Result.failure(RuntimeException("network error"))
        coEvery { pendingRepo.markSynced(10, any()) } returns Unit
        coEvery { pendingRepo.deleteSynced() } returns Unit

        worker().doWork()

        coVerify { pendingRepo.markSynced(10, any()) }
        coVerify(exactly = 1) { pendingRepo.deleteSynced() }
    }

    @Test
    fun `date is formatted as ISO 8601`() {
        // 1_700_000_000_000 = 2023-11-14T22:13:20Z
        val formatted = worker().formatDateForTest(1_700_000_000_000)
        assertNotNull(formatted)
        org.junit.Assert.assertTrue(formatted.startsWith("2023-11-14T22:13:20"))
    }
}
