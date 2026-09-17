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
 * SmsBackfillWorker builds its own SmsHistoryReader(applicationContext.contentResolver)
 * internally, so its input is controlled via ShadowContentResolver.setCursor
 * against the real applicationContext's resolver (the CallLogSyncWorkerTest
 * pattern). Issue #721: the worker gates on the READ_SMS OS grant, granted by
 * default in setup; the denial path is its own test.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
class SmsBackfillWorkerTest {

    private val context = ApplicationProvider.getApplicationContext<Context>()

    @Before
    fun grantSmsPermission() {
        Shadows.shadowOf(context as Application)
            .grantPermissions(android.Manifest.permission.READ_SMS)
    }

    private fun stubSentFolder(rows: List<Array<Any?>>) {
        val cursor = RoboCursor().apply {
            setColumnNames(listOf(Telephony.Sms.ADDRESS, Telephony.Sms.DATE))
            setResults(rows.toTypedArray())
        }
        Shadows.shadowOf(context.contentResolver).setCursor(Telephony.Sms.Sent.CONTENT_URI, cursor)
    }

    private fun worker(
        pendingInteractions: PendingInteractionRepository,
        contacts: ContactRepository,
        settings: TrackingSettingsRepository,
    ) = SmsBackfillWorker(
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
        coEvery { settings.lastSmsTimestamp() } returns 500L

        val result = worker(pendingInteractions, contacts, settings).doWork()

        assertTrue(result is androidx.work.ListenableWorker.Result.Success)
        coVerify(exactly = 0) { pendingInteractions.recordIfNew(any()) }
        coVerify(exactly = 0) { settings.setLastSmsTimestamp(any()) }
    }

    @Test
    fun `an empty Sent folder is a no-op and does not advance the watermark`() = runTest {
        stubSentFolder(emptyList())
        val pendingInteractions = mockk<PendingInteractionRepository>(relaxed = true)
        val contacts = mockk<ContactRepository>(relaxed = true)
        val settings = mockk<TrackingSettingsRepository>()
        coEvery { settings.smsTrackingEnabled() } returns true
        coEvery { settings.lastSmsTimestamp() } returns 500L

        worker(pendingInteractions, contacts, settings).doWork()

        coVerify(exactly = 0) { settings.setLastSmsTimestamp(any()) }
    }

    @Test
    fun `records a matched outgoing text, filters the unmatched one, and advances the watermark`() = runTest {
        stubSentFolder(
            listOf(
                arrayOf<Any?>("+15551234567", 6000L),
                arrayOf<Any?>("+15559876543", 8000L),
            ),
        )
        val pendingInteractions = mockk<PendingInteractionRepository>(relaxed = true)
        val contacts = mockk<ContactRepository>()
        coEvery { contacts.findByPhone("+15551234567") } returns ContactSummary(id = 11)
        coEvery { contacts.findByPhone("+15559876543") } returns null
        val settings = mockk<TrackingSettingsRepository>(relaxed = true)
        coEvery { settings.smsTrackingEnabled() } returns true
        coEvery { settings.lastSmsTimestamp() } returns 0L

        val result = worker(pendingInteractions, contacts, settings).doWork()

        assertTrue(result is androidx.work.ListenableWorker.Result.Success)
        coVerify {
            pendingInteractions.recordIfNew(
                PendingInteraction(
                    timestampMillis = 6000L,
                    kind = InteractionCapture.KIND_MESSAGE,
                    direction = InteractionCapture.DIR_OUTGOING,
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
        // The watermark still advances past everything seen, filtered or not.
        coVerify { settings.setLastSmsTimestamp(8000L) }
    }

    @Test
    fun `the include-unknown opt-in stages an unmatched outgoing text`() = runTest {
        stubSentFolder(listOf(arrayOf<Any?>("+15559876543", 8000L)))
        val pendingInteractions = mockk<PendingInteractionRepository>(relaxed = true)
        val contacts = mockk<ContactRepository>()
        coEvery { contacts.findByPhone("+15559876543") } returns null
        val settings = mockk<TrackingSettingsRepository>(relaxed = true)
        coEvery { settings.smsTrackingEnabled() } returns true
        coEvery { settings.lastSmsTimestamp() } returns 0L
        coEvery { settings.includeUnknownNumbers() } returns true

        worker(pendingInteractions, contacts, settings).doWork()

        coVerify {
            pendingInteractions.recordIfNew(
                PendingInteraction(
                    timestampMillis = 8000L,
                    kind = InteractionCapture.KIND_MESSAGE,
                    direction = InteractionCapture.DIR_OUTGOING,
                    phoneNumber = "+15559876543",
                    matchedContactId = null,
                ),
            )
        }
        coVerify(exactly = 0) { settings.incrementFilteredUnknownCount() }
    }

    @Test
    fun `a null-address row is skipped`() = runTest {
        stubSentFolder(listOf(arrayOf<Any?>(null, 3000L)))
        val pendingInteractions = mockk<PendingInteractionRepository>(relaxed = true)
        val contacts = mockk<ContactRepository>(relaxed = true)
        val settings = mockk<TrackingSettingsRepository>(relaxed = true)
        coEvery { settings.smsTrackingEnabled() } returns true
        coEvery { settings.lastSmsTimestamp() } returns 0L

        worker(pendingInteractions, contacts, settings).doWork()

        coVerify(exactly = 0) { pendingInteractions.recordIfNew(any()) }
    }

    /**
     * Regression test for #1123. RoboCursor/setCursor always returns the same
     * static cursor for a URI, which can't model a real backlog where a second
     * query (after the watermark advances) must return a *different* page —
     * so this stubs a real ContentProvider that serves `date > since ORDER BY
     * date ASC LIMIT n` against an in-memory table, mirroring what the real
     * SMS provider does. Hand-verified: reverting SmsHistoryReader's sort back
     * to `DATE DESC` makes this test fail (only the newest 50 of the 75 rows
     * get recorded, and the oldest row is never seen).
     */
    private class FakeSentSmsProvider(private val data: List<Pair<String, Long>>) : ContentProvider() {

        override fun onCreate() = true

        override fun query(
            uri: Uri,
            projection: Array<out String>?,
            selection: String?,
            selectionArgs: Array<out String>?,
            sortOrder: String?,
        ): Cursor {
            // Honors ASC/DESC from sortOrder like a real SQL provider would, so this
            // fake is sensitive to a regression back to the old DESC query (which
            // would return the newest `limit` rows instead of the oldest).
            val since = selectionArgs?.firstOrNull()?.toLongOrNull() ?: 0L
            val descending = sortOrder?.contains("DESC") == true
            val limit = sortOrder?.substringAfterLast("LIMIT ")?.trim()?.toIntOrNull() ?: Int.MAX_VALUE
            val matching = data.filter { it.second > since }
            val ordered = if (descending) matching.sortedByDescending { it.second } else matching.sortedBy { it.second }
            val page = ordered.take(limit)
            return MatrixCursor(arrayOf(Telephony.Sms.ADDRESS, Telephony.Sms.DATE)).apply {
                page.forEach { (address, date) -> addRow(arrayOf<Any?>(address, date)) }
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
        // 75 rows, oldest at 1_000L, newest at 75_000L — more than the 50-row page size.
        val rows = (1..75).map { i -> "+1555000${i.toString().padStart(4, '0')}" to i * 1_000L }
        val provider = FakeSentSmsProvider(rows)
        provider.onCreate()
        ShadowContentResolver.registerProviderInternal("sms", provider)

        val pendingInteractions = mockk<PendingInteractionRepository>(relaxed = true)
        val contacts = mockk<ContactRepository>(relaxed = true)
        coEvery { contacts.findByPhone(any()) } returns ContactSummary(id = 1)
        val settings = mockk<TrackingSettingsRepository>(relaxed = true)
        coEvery { settings.smsTrackingEnabled() } returns true
        coEvery { settings.lastSmsTimestamp() } returns 0L

        worker(pendingInteractions, contacts, settings).doWork()

        // Every row — including the oldest, which a DESC/newest-50 query would
        // have permanently skipped — gets staged.
        coVerify(exactly = 75) { pendingInteractions.recordIfNew(any()) }
        coVerify { pendingInteractions.recordIfNew(match { it.timestampMillis == 1_000L }) }
        coVerify { pendingInteractions.recordIfNew(match { it.timestampMillis == 75_000L }) }
        coVerify { settings.setLastSmsTimestamp(75_000L) }
    }
}
