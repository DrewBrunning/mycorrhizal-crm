package com.mycorrhizal.crm.feature.tracking

import android.app.Application
import android.content.Context
import android.content.Intent
import android.provider.Telephony
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.PendingInteraction
import com.mycorrhizal.crm.domain.repository.PendingInteractionRepository
import com.mycorrhizal.crm.domain.repository.TrackingSettingsRepository
import com.mycorrhizal.crm.model.network.ContactSummary
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/**
 * SmsReceiver's manifest path resolves its dependencies from the app's Hilt
 * entry point, which requires a real @HiltAndroidApp Application no test in
 * this repo boots (issue #327). These tests therefore construct the receiver
 * through its internal constructor with mocked repositories and a fake parse
 * step, so onReceive is driven directly and deterministically. A
 * StandardTestDispatcher sharing the test scheduler is injected so the
 * fire-and-forget capture coroutine completes before each assertion.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
class SmsReceiverTest {

    private val context = ApplicationProvider.getApplicationContext<Context>()

    private fun smsIntent(): Intent =
        Intent(Telephony.Sms.Intents.SMS_RECEIVED_ACTION)

    private fun buildReceiver(
        scope: TestScope,
        pendingInteractions: PendingInteractionRepository = mockk(relaxed = true),
        contacts: ContactRepository = mockk(relaxed = true),
        settings: TrackingSettingsRepository = mockk(relaxed = true),
        parseSms: (Intent) -> SmsEntry? = { null },
    ) = SmsReceiver(
        pendingInteractionRepository = pendingInteractions,
        contactRepository = contacts,
        trackingSettings = settings,
        dispatcher = StandardTestDispatcher(scope.testScheduler),
        parseSms = parseSms,
    )

    @Test
    fun `a non-SMS broadcast action is ignored without touching any dependency`() = runTest {
        val pending = mockk<PendingInteractionRepository>(relaxed = true)
        val contacts = mockk<ContactRepository>(relaxed = true)
        val settings = mockk<TrackingSettingsRepository>(relaxed = true)
        val receiver = buildReceiver(this, pending, contacts, settings)

        receiver.onReceive(context, Intent("some.other.action"))
        testScheduler.advanceUntilIdle()

        coVerify(exactly = 0) { pending.record(any()) }
        coVerify(exactly = 0) { contacts.findByPhone(any()) }
        coVerify(exactly = 0) { settings.smsTrackingEnabled() }
    }

    @Test
    fun `an SMS broadcast with no parseable message is ignored`() = runTest {
        val pending = mockk<PendingInteractionRepository>(relaxed = true)
        val contacts = mockk<ContactRepository>(relaxed = true)
        val settings = mockk<TrackingSettingsRepository>(relaxed = true)
        coEvery { settings.smsTrackingEnabled() } returns true
        val receiver = buildReceiver(this, pending, contacts, settings)

        receiver.onReceive(context, smsIntent())
        testScheduler.advanceUntilIdle()

        coVerify(exactly = 0) { pending.record(any()) }
        coVerify(exactly = 0) { contacts.findByPhone(any()) }
    }

    @Test
    fun `an SMS broadcast while sms tracking is disabled records nothing`() = runTest {
        val pending = mockk<PendingInteractionRepository>(relaxed = true)
        val contacts = mockk<ContactRepository>(relaxed = true)
        val settings = mockk<TrackingSettingsRepository>(relaxed = true)
        coEvery { settings.smsTrackingEnabled() } returns false
        val receiver = buildReceiver(
            this,
            pending,
            contacts,
            settings,
            parseSms = { SmsEntry(address = "+15551234567", body = "hello", timestampMillis = 1234L) },
        )

        receiver.onReceive(context, smsIntent())
        testScheduler.advanceUntilIdle()

        coVerify(exactly = 0) { pending.record(any()) }
        coVerify(exactly = 0) { contacts.findByPhone(any()) }
    }

    @Test
    fun `records an unmatched SMS as a pending interaction`() = runTest {
        val pending = mockk<PendingInteractionRepository>(relaxed = true)
        val contacts = mockk<ContactRepository>(relaxed = true)
        val settings = mockk<TrackingSettingsRepository>(relaxed = true)
        coEvery { settings.smsTrackingEnabled() } returns true
        coEvery { contacts.findByPhone("+15551234567") } returns null
        val receiver = buildReceiver(
            this,
            pending,
            contacts,
            settings,
            parseSms = { SmsEntry(address = "+15551234567", body = "hello", timestampMillis = 1234L) },
        )

        receiver.onReceive(context, smsIntent())
        testScheduler.advanceUntilIdle()

        coVerify { contacts.findByPhone("+15551234567") }
        coVerify {
            pending.record(
                PendingInteraction(
                    timestampMillis = 1234L,
                    kind = InteractionCapture.KIND_MESSAGE,
                    direction = InteractionCapture.DIR_INCOMING,
                    phoneNumber = "+15551234567",
                    matchedContactId = null,
                ),
            )
        }
    }

    @Test
    fun `records a matched SMS with the matched contact id`() = runTest {
        val pending = mockk<PendingInteractionRepository>(relaxed = true)
        val contacts = mockk<ContactRepository>(relaxed = true)
        val settings = mockk<TrackingSettingsRepository>(relaxed = true)
        coEvery { settings.smsTrackingEnabled() } returns true
        coEvery { contacts.findByPhone("+15551234567") } returns ContactSummary(id = 9)
        val receiver = buildReceiver(
            this,
            pending,
            contacts,
            settings,
            parseSms = { SmsEntry(address = "+15551234567", body = "hello", timestampMillis = 1234L) },
        )

        receiver.onReceive(context, smsIntent())
        testScheduler.advanceUntilIdle()

        coVerify {
            pending.record(
                PendingInteraction(
                    timestampMillis = 1234L,
                    kind = InteractionCapture.KIND_MESSAGE,
                    direction = InteractionCapture.DIR_INCOMING,
                    phoneNumber = "+15551234567",
                    matchedContactId = 9,
                ),
            )
        }
    }

    @Test
    fun `a contact lookup failure still records the interaction unmatched`() = runTest {
        val pending = mockk<PendingInteractionRepository>(relaxed = true)
        val contacts = mockk<ContactRepository>(relaxed = true)
        val settings = mockk<TrackingSettingsRepository>(relaxed = true)
        coEvery { settings.smsTrackingEnabled() } returns true
        coEvery { contacts.findByPhone("+15551234567") } throws RuntimeException("db error")
        val receiver = buildReceiver(
            this,
            pending,
            contacts,
            settings,
            parseSms = { SmsEntry(address = "+15551234567", body = "hello", timestampMillis = 1234L) },
        )

        receiver.onReceive(context, smsIntent())
        testScheduler.advanceUntilIdle()

        coVerify { contacts.findByPhone("+15551234567") }
        coVerify {
            pending.record(
                PendingInteraction(
                    timestampMillis = 1234L,
                    kind = InteractionCapture.KIND_MESSAGE,
                    direction = InteractionCapture.DIR_INCOMING,
                    phoneNumber = "+15551234567",
                    matchedContactId = null,
                ),
            )
        }
    }

    @Test
    fun `an SMS with no address skips the phone match but is still recorded`() = runTest {
        val pending = mockk<PendingInteractionRepository>(relaxed = true)
        val contacts = mockk<ContactRepository>(relaxed = true)
        val settings = mockk<TrackingSettingsRepository>(relaxed = true)
        coEvery { settings.smsTrackingEnabled() } returns true
        val receiver = buildReceiver(
            this,
            pending,
            contacts,
            settings,
            parseSms = { SmsEntry(address = null, body = "hello", timestampMillis = 5555L) },
        )

        receiver.onReceive(context, smsIntent())
        testScheduler.advanceUntilIdle()

        coVerify(exactly = 0) { contacts.findByPhone(any()) }
        coVerify {
            pending.record(
                PendingInteraction(
                    timestampMillis = 5555L,
                    kind = InteractionCapture.KIND_MESSAGE,
                    direction = InteractionCapture.DIR_INCOMING,
                    phoneNumber = null,
                    matchedContactId = null,
                ),
            )
        }
    }
}
