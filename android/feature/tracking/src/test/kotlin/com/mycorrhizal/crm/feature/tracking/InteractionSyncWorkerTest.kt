package com.mycorrhizal.crm.feature.tracking

import com.mycorrhizal.crm.domain.repository.PendingInteraction
import com.mycorrhizal.crm.domain.repository.PendingInteractionRepository
import com.mycorrhizal.crm.domain.repository.TrackingSettingsRepository
import com.mycorrhizal.crm.model.network.ActivityInput
import com.mycorrhizal.crm.network.ApiClient
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

class InteractionSyncWorkerLogicTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val pendingRepo = mockk<PendingInteractionRepository>()
    private val trackingSettings = mockk<TrackingSettingsRepository>()
    private val apiClient = mockk<ApiClient>()

    @Test
    fun `call interaction maps to a call activity with the matched contact`() = runTest {
        coEvery { trackingSettings.callTrackingEnabled() } returns true
        coEvery { trackingSettings.smsTrackingEnabled() } returns true
        coEvery { pendingRepo.unsynced() } returns listOf(
            PendingInteraction(
                id = 1,
                timestampMillis = 1_700_000_000_000,
                kind = InteractionCapture.KIND_CALL,
                direction = InteractionCapture.DIR_INCOMING,
                phoneNumber = "+15550100",
                matchedContactId = 7,
            ),
        )
        coEvery {
            apiClient.createActivity(
                match { it.type == "call" && it.title == "Call" && it.contactIds == listOf(7) },
            )
        } returns Result.success(com.mycorrhizal.crm.model.network.Activity(id = 99))
        coEvery { pendingRepo.markSynced(1, any()) } returns Unit
        coEvery { pendingRepo.deleteSynced() } returns Unit
        coEvery { trackingSettings.lastCallLogTimestamp() } returns 0L
        coEvery { trackingSettings.setLastCallLogTimestamp(any()) } returns Unit

        val worker = InteractionSyncWorker(
            appContext = mockk(relaxed = true),
            workerParams = mockk(relaxed = true),
            pendingInteractionRepository = pendingRepo,
            trackingSettings = trackingSettings,
            apiClient = apiClient,
        )
        val result = worker.doWork()

        assertEquals(androidx.work.ListenableWorker.Result.success(), result)
        coVerify { pendingRepo.markSynced(1, any()) }
    }

    @Test
    fun `missed call interaction maps to a missed-call title`() = runTest {
        coEvery { trackingSettings.callTrackingEnabled() } returns true
        coEvery { trackingSettings.smsTrackingEnabled() } returns true
        coEvery { pendingRepo.unsynced() } returns listOf(
            PendingInteraction(
                id = 2,
                timestampMillis = 1_700_000_000_000,
                kind = InteractionCapture.KIND_CALL,
                direction = InteractionCapture.DIR_MISSED,
                phoneNumber = "+15550100",
            ),
        )
        coEvery {
            apiClient.createActivity(
                match { it.type == "call" && it.title == "Missed call" },
            )
        } returns Result.success(com.mycorrhizal.crm.model.network.Activity(id = 100))
        coEvery { pendingRepo.markSynced(2, any()) } returns Unit
        coEvery { pendingRepo.deleteSynced() } returns Unit
        coEvery { trackingSettings.lastCallLogTimestamp() } returns 0L
        coEvery { trackingSettings.setLastCallLogTimestamp(any()) } returns Unit

        val worker = InteractionSyncWorker(
            appContext = mockk(relaxed = true),
            workerParams = mockk(relaxed = true),
            pendingInteractionRepository = pendingRepo,
            trackingSettings = trackingSettings,
            apiClient = apiClient,
        )
        worker.doWork()
        coVerify { pendingRepo.markSynced(2, any()) }
    }

    @Test
    fun `message interaction maps to a message activity with no description`() = runTest {
        coEvery { trackingSettings.callTrackingEnabled() } returns true
        coEvery { trackingSettings.smsTrackingEnabled() } returns true
        coEvery { pendingRepo.unsynced() } returns listOf(
            PendingInteraction(
                id = 3,
                timestampMillis = 1_700_000_000_000,
                kind = InteractionCapture.KIND_MESSAGE,
                direction = InteractionCapture.DIR_INCOMING,
                phoneNumber = "+15550100",
                matchedContactId = 9,
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
            )
        } returns Result.success(com.mycorrhizal.crm.model.network.Activity(id = 101))
        coEvery { pendingRepo.markSynced(3, any()) } returns Unit
        coEvery { pendingRepo.deleteSynced() } returns Unit
        coEvery { trackingSettings.lastCallLogTimestamp() } returns 0L
        coEvery { trackingSettings.setLastCallLogTimestamp(any()) } returns Unit

        val worker = InteractionSyncWorker(
            appContext = mockk(relaxed = true),
            workerParams = mockk(relaxed = true),
            pendingInteractionRepository = pendingRepo,
            trackingSettings = trackingSettings,
            apiClient = apiClient,
        )
        worker.doWork()
        coVerify { pendingRepo.markSynced(3, any()) }
    }

    @Test
    fun `date is formatted as ISO 8601`() {
        // 1_700_000_000_000 = 2023-11-14T22:13:20Z
        val worker = InteractionSyncWorker(
            appContext = mockk(relaxed = true),
            workerParams = mockk(relaxed = true),
            pendingInteractionRepository = pendingRepo,
            trackingSettings = trackingSettings,
            apiClient = apiClient,
        )
        val formatted = worker.formatDateForTest(1_700_000_000_000)
        assertNotNull(formatted)
        org.junit.Assert.assertTrue(formatted.startsWith("2023-11-14T22:13:20"))
    }
}
