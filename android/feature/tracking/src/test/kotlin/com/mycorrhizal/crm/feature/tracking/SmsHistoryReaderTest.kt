package com.mycorrhizal.crm.feature.tracking

import android.content.ContentResolver
import android.database.MatrixCursor
import android.provider.Telephony
import io.mockk.every
import io.mockk.mockk
import io.mockk.slot
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
class SmsHistoryReaderTest {

    private val contentResolver = mockk<ContentResolver>()
    private val reader = SmsHistoryReader(contentResolver)

    private val projection = arrayOf(Telephony.Sms.ADDRESS, Telephony.Sms.DATE)

    @Test
    fun `readSentSince maps cursor rows to SmsHistoryEntry`() {
        val cursor = MatrixCursor(projection).apply {
            addRow(arrayOf<Any>("+15551234567", 6_000L))
            addRow(arrayOf<Any>("+15559876543", 8_000L))
        }
        every {
            contentResolver.query(Telephony.Sms.Sent.CONTENT_URI, projection, any(), any(), any())
        } returns cursor

        val entries = reader.readSentSince(sinceMillis = 0L)

        assertEquals(2, entries.size)
        assertEquals("+15551234567", entries[0].address)
        assertEquals(6_000L, entries[0].timestampMillis)
        assertEquals("+15559876543", entries[1].address)
        assertEquals(8_000L, entries[1].timestampMillis)
    }

    @Test
    fun `a missing ADDRESS column maps to a null address`() {
        val cursor = MatrixCursor(arrayOf(Telephony.Sms.DATE)).apply {
            addRow(arrayOf<Any>(9_000L))
        }
        every {
            contentResolver.query(Telephony.Sms.Sent.CONTENT_URI, projection, any(), any(), any())
        } returns cursor

        val entries = reader.readSentSince(sinceMillis = 0L)

        assertNull(entries.single().address)
        assertEquals(9_000L, entries.single().timestampMillis)
    }

    @Test
    fun `a null cursor from the provider is an empty list`() {
        every {
            contentResolver.query(Telephony.Sms.Sent.CONTENT_URI, projection, any(), any(), any())
        } returns null

        val entries = reader.readSentSince(sinceMillis = 0L)

        assertTrue(entries.isEmpty())
    }

    @Test
    fun `queries the Sent folder oldest-first so a caller can page forward through a backlog`() {
        // Regression test for #1123: a DESC/newest-first query made the caller's
        // watermark jump straight to "now" whenever more than `limit` rows existed
        // past it, permanently skipping everything older than the newest `limit`.
        val cursor = MatrixCursor(projection)
        val sortOrder = slot<String>()
        every {
            contentResolver.query(Telephony.Sms.Sent.CONTENT_URI, projection, any(), any(), capture(sortOrder))
        } returns cursor

        reader.readSentSince(sinceMillis = 0L, limit = 50)

        assertEquals("${Telephony.Sms.DATE} ASC LIMIT 50", sortOrder.captured)
    }

    @Test
    fun `a missing READ_SMS grant (SecurityException) is an empty list, not a crash`() {
        every {
            contentResolver.query(Telephony.Sms.Sent.CONTENT_URI, projection, any(), any(), any())
        } throws SecurityException("Permission Denial: reading com.android.providers.telephony")

        val entries = reader.readSentSince(sinceMillis = 0L)

        assertTrue(entries.isEmpty())
    }
}
