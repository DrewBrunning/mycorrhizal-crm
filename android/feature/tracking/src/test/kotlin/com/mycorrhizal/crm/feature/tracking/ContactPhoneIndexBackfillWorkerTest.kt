package com.mycorrhizal.crm.feature.tracking

import android.app.Application
import android.content.Context
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.TrackingSettingsRepository
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/**
 * Issue #1122: a cached contact's non-primary phone numbers can't match an
 * incoming SMS/call until its detail screen has been opened at least once,
 * since a list-sync row only ever carries the primary phone. This worker
 * hydrates a bounded batch of contacts still missing a full detail fetch.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
class ContactPhoneIndexBackfillWorkerTest {

    private val context = ApplicationProvider.getApplicationContext<Context>()

    private fun worker(contacts: ContactRepository, settings: TrackingSettingsRepository) =
        ContactPhoneIndexBackfillWorker(
            appContext = context,
            workerParams = mockk(relaxed = true),
            contactRepository = contacts,
            trackingSettings = settings,
        )

    @Test
    fun `a no-op when neither capture path is enabled`() = runTest {
        val contacts = mockk<ContactRepository>(relaxed = true)
        val settings = mockk<TrackingSettingsRepository>()
        coEvery { settings.smsTrackingEnabled() } returns false
        coEvery { settings.callTrackingEnabled() } returns false

        val result = worker(contacts, settings).doWork()

        assertTrue(result is androidx.work.ListenableWorker.Result.Success)
        coVerify(exactly = 0) { contacts.getContactIdsMissingPhoneIndex(any()) }
    }

    @Test
    fun `hydrates every contact missing a full detail fetch`() = runTest {
        val contacts = mockk<ContactRepository>()
        val settings = mockk<TrackingSettingsRepository>()
        coEvery { settings.smsTrackingEnabled() } returns true
        coEvery { settings.callTrackingEnabled() } returns false
        coEvery { contacts.getContactIdsMissingPhoneIndex(any()) } returns listOf(1, 2, 3)
        coEvery { contacts.getContact(any()) } returns Result.success(ContactRecordResponse(id = 1))

        val result = worker(contacts, settings).doWork()

        assertTrue(result is androidx.work.ListenableWorker.Result.Success)
        coVerify(exactly = 1) { contacts.getContact(1) }
        coVerify(exactly = 1) { contacts.getContact(2) }
        coVerify(exactly = 1) { contacts.getContact(3) }
    }

    @Test
    fun `runs when only call tracking is enabled`() = runTest {
        val contacts = mockk<ContactRepository>()
        val settings = mockk<TrackingSettingsRepository>()
        coEvery { settings.smsTrackingEnabled() } returns false
        coEvery { settings.callTrackingEnabled() } returns true
        coEvery { contacts.getContactIdsMissingPhoneIndex(any()) } returns emptyList()

        val result = worker(contacts, settings).doWork()

        assertTrue(result is androidx.work.ListenableWorker.Result.Success)
        coVerify(exactly = 1) { contacts.getContactIdsMissingPhoneIndex(any()) }
    }

    @Test
    fun `a fetch failure for one contact does not stop the run or fail the worker`() = runTest {
        val contacts = mockk<ContactRepository>()
        val settings = mockk<TrackingSettingsRepository>()
        coEvery { settings.smsTrackingEnabled() } returns true
        coEvery { settings.callTrackingEnabled() } returns false
        coEvery { contacts.getContactIdsMissingPhoneIndex(any()) } returns listOf(1, 2)
        coEvery { contacts.getContact(1) } throws RuntimeException("network down")
        coEvery { contacts.getContact(2) } returns Result.success(ContactRecordResponse(id = 2))

        val result = worker(contacts, settings).doWork()

        assertTrue(result is androidx.work.ListenableWorker.Result.Success)
        coVerify(exactly = 1) { contacts.getContact(2) }
    }
}
