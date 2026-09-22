package com.mycorrhizal.crm.feature.tracking

import android.app.Application
import android.content.ContentProvider
import android.content.ContentValues
import android.content.Context
import android.database.Cursor
import android.database.MatrixCursor
import android.net.Uri
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
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.Shadows
import org.robolectric.annotation.Config
import org.robolectric.fakes.RoboCursor
import org.robolectric.shadows.ShadowContentResolver

/**
 * ADR 0019 / issue #1127: the reconciliation worker's own tests, mirroring
 * SmsBackfillWorkerTest's shape (same problem shape -- a periodic provider
 * read gated on opt-in + READ_SMS, paged ASC, capturing through the shared
 * InteractionCapture policy) with Inbox/`_id` swapped in for Sent/`DATE`.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
class SmsInboxReconciliationWorkerTest {

    private val context = ApplicationProvider.getApplicationContext<Context>()

    @Before
    fun grantSmsPermission() {
        Shadows.shadowOf(context as Application)
            .grantPermissions(android.Manifest.permission.READ_SMS)
    }

    private fun stubInbox(rows: List<Array<Any?>>) {
        val cursor = RoboCursor().apply {
            setColumnNames(listOf(Telephony.Sms._ID, Telephony.Sms.ADDRESS, Telephony.Sms.DATE))
            setResults(rows.toTypedArray())
        }
        Shadows.shadowOf(context.contentResolver).setCursor(Telephony.Sms.Inbox.CONTENT_URI, cursor)
    }

    private fun worker(
        pendingInteractions: PendingInteractionRepository,
        contacts: ContactRepository,
        settings: TrackingSettingsRepository,
    ) = SmsInboxReconciliationWorker(
        appContext = context,
        workerParams = mockk(relaxed = true),
        pendingInteractionRepository = pendingInteractions,
        contactRepository = contacts,
        trackingSettings = settings,
    )

    @Test
    fun `a disabled SMS opt-in short-circuits without reading the provider`() = runTest {
        val pendingInteractions = mockk<PendingInteractionRepository>(relaxed = true)
        val contacts = mockk<ContactRepository>(relaxed = true)
        val settings = mockk<TrackingSettingsRepository>()
        coEvery { settings.smsTrackingEnabled() } returns false

        val result = worker(pendingInteractions, contacts, settings).doWork()

        assertTrue(result is androidx.work.ListenableWorker.Result.Success)
        coVerify(exactly = 0) { pendingInteractions.recordIfNew(any()) }
    }

    @Test
    fun `a missing READ_SMS grant is a no-op success, not a worker failure`() = runTest {
        Shadows.shadowOf(context as Application).denyPermissions(android.Manifest.permission.READ_SMS)
        val pendingInteractions = mockk<PendingInteractionRepository>(relaxed = true)
        val contacts = mockk<ContactRepository>(relaxed = true)
        val settings = mockk<TrackingSettingsRepository>()
        coEvery { settings.smsTrackingEnabled() } returns true
        coEvery { settings.lastSmsInboxId() } returns 500L

        val result = worker(pendingInteractions, contacts, settings).doWork()

        assertTrue(result is androidx.work.ListenableWorker.Result.Success)
        coVerify(exactly = 0) { pendingInteractions.recordIfNew(any()) }
        coVerify(exactly = 0) { settings.setLastSmsInboxId(any()) }
    }

    @Test
    fun `an empty Inbox is a no-op and does not advance the cursor`() = runTest {
        stubInbox(emptyList())
        val pendingInteractions = mockk<PendingInteractionRepository>(relaxed = true)
        val contacts = mockk<ContactRepository>(relaxed = true)
        val settings = mockk<TrackingSettingsRepository>()
        coEvery { settings.smsTrackingEnabled() } returns true
        coEvery { settings.lastSmsInboxId() } returns 500L

        worker(pendingInteractions, contacts, settings).doWork()

        coVerify(exactly = 0) { settings.setLastSmsInboxId(any()) }
    }

    @Test
    fun `records a matched incoming text, filters the unmatched one, and advances the cursor`() = runTest {
        stubInbox(
            listOf(
                arrayOf<Any?>(6L, "+15551234567", 6000L),
                arrayOf<Any?>(8L, "+15559876543", 8000L),
            ),
        )
        val pendingInteractions = mockk<PendingInteractionRepository>(relaxed = true)
        val contacts = mockk<ContactRepository>()
        coEvery { contacts.findByPhone("+15551234567") } returns ContactSummary(id = 11)
        coEvery { contacts.findByPhone("+15559876543") } returns null
        val settings = mockk<TrackingSettingsRepository>(relaxed = true)
        coEvery { settings.smsTrackingEnabled() } returns true
        coEvery { settings.lastSmsInboxId() } returns 0L

        val result = worker(pendingInteractions, contacts, settings).doWork()

        assertTrue(result is androidx.work.ListenableWorker.Result.Success)
        coVerify {
            pendingInteractions.recordIfNew(
                PendingInteraction(
                    timestampMillis = 6000L,
                    kind = InteractionCapture.KIND_MESSAGE,
                    direction = InteractionCapture.DIR_INCOMING,
                    phoneNumber = "+15551234567",
                    matchedContactId = 11,
                ),
            )
        }
        // The unmatched row is dropped (issue #1029), never staged.
        coVerify(exactly = 0) {
            pendingInteractions.recordIfNew(match { it.phoneNumber == "+15559876543" })
        }
        coVerify(exactly = 1) { settings.incrementFilteredUnknownCount() }
        // The cursor still advances past everything seen, filtered or not.
        coVerify { settings.setLastSmsInboxId(8L) }
    }

    @Test
    fun `the include-unknown opt-in stages an unmatched incoming text`() = runTest {
        stubInbox(listOf(arrayOf<Any?>(8L, "+15559876543", 8000L)))
        val pendingInteractions = mockk<PendingInteractionRepository>(relaxed = true)
        val contacts = mockk<ContactRepository>()
        coEvery { contacts.findByPhone("+15559876543") } returns null
        val settings = mockk<TrackingSettingsRepository>(relaxed = true)
        coEvery { settings.smsTrackingEnabled() } returns true
        coEvery { settings.lastSmsInboxId() } returns 0L
        coEvery { settings.includeUnknownNumbers() } returns true

        worker(pendingInteractions, contacts, settings).doWork()

        coVerify {
            pendingInteractions.recordIfNew(
                PendingInteraction(
                    timestampMillis = 8000L,
                    kind = InteractionCapture.KIND_MESSAGE,
                    direction = InteractionCapture.DIR_INCOMING,
                    phoneNumber = "+15559876543",
                    matchedContactId = null,
                ),
            )
        }
        coVerify(exactly = 0) { settings.incrementFilteredUnknownCount() }
    }

    @Test
    fun `a null-address row is skipped`() = runTest {
        stubInbox(listOf(arrayOf<Any?>(3L, null, 3000L)))
        val pendingInteractions = mockk<PendingInteractionRepository>(relaxed = true)
        val contacts = mockk<ContactRepository>(relaxed = true)
        val settings = mockk<TrackingSettingsRepository>(relaxed = true)
        coEvery { settings.smsTrackingEnabled() } returns true
        coEvery { settings.lastSmsInboxId() } returns 0L

        worker(pendingInteractions, contacts, settings).doWork()

        coVerify(exactly = 0) { pendingInteractions.recordIfNew(any()) }
    }

    @Test
    fun `no cursor set yet (null) starts reading from the beginning`() = runTest {
        stubInbox(listOf(arrayOf<Any?>(1L, "+15551234567", 1000L)))
        val pendingInteractions = mockk<PendingInteractionRepository>(relaxed = true)
        val contacts = mockk<ContactRepository>(relaxed = true)
        coEvery { contacts.findByPhone(any()) } returns ContactSummary(id = 1)
        val settings = mockk<TrackingSettingsRepository>(relaxed = true)
        coEvery { settings.smsTrackingEnabled() } returns true
        coEvery { settings.lastSmsInboxId() } returns null

        worker(pendingInteractions, contacts, settings).doWork()

        coVerify(exactly = 1) { pendingInteractions.recordIfNew(any()) }
        coVerify { settings.setLastSmsInboxId(1L) }
    }

    /**
     * Regression test for #1123's lesson applied to this reader (ADR 0019 /
     * #1127): RoboCursor/setCursor always returns the same static cursor for
     * a URI, which can't model a real backlog where a second query (after the
     * cursor advances) must return a *different* page -- so this stubs a real
     * ContentProvider that serves `_id > since ORDER BY _id ASC LIMIT n`
     * against an in-memory table, mirroring what the real SMS provider does.
     * Hand-verified: reverting SmsInboxReader's sort back to `_ID DESC` makes
     * this test fail (only the newest 50 of the 75 rows get recorded, and the
     * oldest row is never seen).
     */
    private class FakeInboxSmsProvider(private val data: List<Triple<Long, String, Long>>) : ContentProvider() {

        override fun onCreate() = true

        override fun query(
            uri: Uri,
            projection: Array<out String>?,
            selection: String?,
            selectionArgs: Array<out String>?,
            sortOrder: String?,
        ): Cursor {
            // Honors ASC/DESC from sortOrder like a real SQL provider would, so this
            // fake is sensitive to a regression back to the old DESC query.
            val since = selectionArgs?.firstOrNull()?.toLongOrNull() ?: 0L
            val descending = sortOrder?.contains("DESC") == true
            val limit = sortOrder?.substringAfterLast("LIMIT ")?.trim()?.toIntOrNull() ?: Int.MAX_VALUE
            val matching = data.filter { it.first > since }
            val ordered = if (descending) matching.sortedByDescending { it.first } else matching.sortedBy { it.first }
            val page = ordered.take(limit)
            return MatrixCursor(arrayOf(Telephony.Sms._ID, Telephony.Sms.ADDRESS, Telephony.Sms.DATE)).apply {
                page.forEach { (id, address, date) -> addRow(arrayOf<Any?>(id, address, date)) }
            }
        }

        override fun getType(uri: Uri): String? = null
        override fun insert(uri: Uri, values: ContentValues?): Uri? = null
        override fun delete(uri: Uri, selection: String?, selectionArgs: Array<out String>?): Int = 0
        override fun update(
            uri: Uri,
            values: ContentValues?,
            selection: String?,
            selectionArgs: Array<out String>?,
        ): Int = 0
    }

    @Test
    fun `pages through a backlog of more than one page instead of skipping the older half`() = runTest {
        // 75 rows, oldest _id 1, newest _id 75 -- more than the 50-row page size.
        val rows = (1..75).map { i -> Triple(i.toLong(), "+1555000${i.toString().padStart(4, '0')}", i * 1_000L) }
        val provider = FakeInboxSmsProvider(rows)
        provider.onCreate()
        ShadowContentResolver.registerProviderInternal("sms", provider)

        val pendingInteractions = mockk<PendingInteractionRepository>(relaxed = true)
        val contacts = mockk<ContactRepository>(relaxed = true)
        coEvery { contacts.findByPhone(any()) } returns ContactSummary(id = 1)
        val settings = mockk<TrackingSettingsRepository>(relaxed = true)
        coEvery { settings.smsTrackingEnabled() } returns true
        coEvery { settings.lastSmsInboxId() } returns 0L

        worker(pendingInteractions, contacts, settings).doWork()

        // Every row -- including the oldest, which a DESC/newest-50 query would
        // have permanently skipped -- gets staged.
        coVerify(exactly = 75) { pendingInteractions.recordIfNew(any()) }
        coVerify { pendingInteractions.recordIfNew(match { it.timestampMillis == 1_000L }) }
        coVerify { pendingInteractions.recordIfNew(match { it.timestampMillis == 75_000L }) }
        coVerify { settings.setLastSmsInboxId(75L) }
    }
}
