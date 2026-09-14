package com.mycorrhizal.crm.feature.tracking

import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.PendingInteraction
import com.mycorrhizal.crm.domain.repository.PendingInteractionRepository
import com.mycorrhizal.crm.domain.repository.TrackingSettingsRepository
import com.mycorrhizal.crm.model.network.ContactSummary
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * Issue #1029: the single capture-policy entry point. SmsReceiver,
 * SmsBackfillWorker and CallLogSyncWorker all delegate here, so the
 * known-contacts-only filter, its escape hatch, the dropped-count, and the
 * record-vs-recordIfNew choice are pinned in one place; each capture site's own
 * test additionally proves it routes through this.
 */
class InteractionCaptureTest {

    private val contacts = mockk<ContactRepository>()
    private val outbox = mockk<PendingInteractionRepository>(relaxed = true)
    private val settings = mockk<TrackingSettingsRepository>(relaxed = true)

    private suspend fun capture(
        number: String?,
        dedupe: Boolean = false,
    ): InteractionCapture.Outcome = InteractionCapture.capture(
        contactRepository = contacts,
        pendingInteractionRepository = outbox,
        trackingSettings = settings,
        kind = InteractionCapture.KIND_CALL,
        direction = InteractionCapture.DIR_INCOMING,
        number = number,
        timestampMillis = 1234L,
        dedupe = dedupe,
    )

    @Test
    fun `a matched number is staged with the contact id`() = runTest {
        coEvery { contacts.findByPhone("+15551234567") } returns ContactSummary(id = 9)
        coEvery { settings.includeUnknownNumbers() } returns false

        val outcome = capture("+15551234567")

        assertEquals(InteractionCapture.Outcome.STAGED, outcome)
        coVerify {
            outbox.record(
                PendingInteraction(
                    timestampMillis = 1234L,
                    kind = InteractionCapture.KIND_CALL,
                    direction = InteractionCapture.DIR_INCOMING,
                    phoneNumber = "+15551234567",
                    matchedContactId = 9,
                ),
            )
        }
    }

    @Test
    fun `an unmatched number is dropped and counted, never staged`() = runTest {
        coEvery { contacts.findByPhone("+15559876543") } returns null
        coEvery { settings.includeUnknownNumbers() } returns false

        val outcome = capture("+15559876543")

        assertEquals(InteractionCapture.Outcome.FILTERED, outcome)
        coVerify(exactly = 0) { outbox.record(any()) }
        coVerify(exactly = 0) { outbox.recordIfNew(any()) }
        coVerify(exactly = 1) { settings.incrementFilteredUnknownCount() }
    }

    @Test
    fun `a null number is dropped and counted`() = runTest {
        coEvery { settings.includeUnknownNumbers() } returns false

        val outcome = capture(null)

        assertEquals(InteractionCapture.Outcome.FILTERED, outcome)
        coVerify(exactly = 0) { outbox.record(any()) }
        coVerify(exactly = 1) { settings.incrementFilteredUnknownCount() }
    }

    @Test
    fun `the include-unknown escape hatch stages an unmatched number unassociated`() = runTest {
        coEvery { contacts.findByPhone("+15559876543") } returns null
        coEvery { settings.includeUnknownNumbers() } returns true

        val outcome = capture("+15559876543")

        assertEquals(InteractionCapture.Outcome.STAGED, outcome)
        coVerify {
            outbox.record(
                PendingInteraction(
                    timestampMillis = 1234L,
                    kind = InteractionCapture.KIND_CALL,
                    direction = InteractionCapture.DIR_INCOMING,
                    phoneNumber = "+15559876543",
                    matchedContactId = null,
                ),
            )
        }
        coVerify(exactly = 0) { settings.incrementFilteredUnknownCount() }
    }

    @Test
    fun `the escape hatch also stages a null number`() = runTest {
        coEvery { settings.includeUnknownNumbers() } returns true

        val outcome = capture(null)

        assertEquals(InteractionCapture.Outcome.STAGED, outcome)
        coVerify { outbox.record(match { it.phoneNumber == null && it.matchedContactId == null }) }
    }

    @Test
    fun `a lookup failure counts as unknown and is filtered by default`() = runTest {
        coEvery { contacts.findByPhone(any()) } throws RuntimeException("db error")
        coEvery { settings.includeUnknownNumbers() } returns false

        val outcome = capture("+15551234567")

        assertEquals(InteractionCapture.Outcome.FILTERED, outcome)
        coVerify(exactly = 0) { outbox.record(any()) }
        coVerify(exactly = 1) { settings.incrementFilteredUnknownCount() }
    }

    @Test
    fun `dedupe uses recordIfNew, not record`() = runTest {
        coEvery { contacts.findByPhone("+15551234567") } returns ContactSummary(id = 9)
        coEvery { outbox.recordIfNew(any()) } returns true

        val outcome = capture("+15551234567", dedupe = true)

        assertEquals(InteractionCapture.Outcome.STAGED, outcome)
        coVerify(exactly = 1) { outbox.recordIfNew(any()) }
        coVerify(exactly = 0) { outbox.record(any()) }
    }

    @Test
    fun `dedupe reports an existing row without counting it as filtered`() = runTest {
        coEvery { contacts.findByPhone("+15551234567") } returns ContactSummary(id = 9)
        coEvery { outbox.recordIfNew(any()) } returns false

        val outcome = capture("+15551234567", dedupe = true)

        assertEquals(InteractionCapture.Outcome.DUPLICATE, outcome)
        coVerify(exactly = 0) { settings.incrementFilteredUnknownCount() }
    }
}
