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
class SmsInboxReaderTest {

    private val contentResolver = mockk<ContentResolver>()
    private val reader = SmsInboxReader(contentResolver)

    private val idProjection = arrayOf(Telephony.Sms._ID)
    private val readSinceProjection = arrayOf(Telephony.Sms._ID, Telephony.Sms.ADDRESS, Telephony.Sms.DATE)

    // --- findId --------------------------------------------------------

    @Test
    fun `findId returns the matching row's _id`() {
        val cursor = MatrixCursor(idProjection).apply { addRow(arrayOf<Any>(42L)) }
        every {
            contentResolver.query(Telephony.Sms.Inbox.CONTENT_URI, idProjection, any(), any(), null)
        } returns cursor

        val id = reader.findId("+15551234567", 1234L)

        assertEquals(42L, id)
    }

    @Test
    fun `findId matches on address and date`() {
        val selection = slot<String>()
        val selectionArgs = slot<Array<String>>()
        val cursor = MatrixCursor(idProjection).apply { addRow(arrayOf<Any>(1L)) }
        every {
            contentResolver.query(Telephony.Sms.Inbox.CONTENT_URI, idProjection, capture(selection), capture(selectionArgs), null)
        } returns cursor

        reader.findId("+15551234567", 1234L)

        assertEquals("${Telephony.Sms.ADDRESS} = ? AND ${Telephony.Sms.DATE} = ?", selection.captured)
        assertEquals(listOf("+15551234567", "1234"), selectionArgs.captured.toList())
    }

    @Test
    fun `findId returns null when no row matches`() {
        val cursor = MatrixCursor(idProjection)
        every {
            contentResolver.query(Telephony.Sms.Inbox.CONTENT_URI, idProjection, any(), any(), null)
        } returns cursor

        val id = reader.findId("+15551234567", 1234L)

        assertNull(id)
    }

    @Test
    fun `findId returns null on a null cursor from the provider`() {
        every {
            contentResolver.query(Telephony.Sms.Inbox.CONTENT_URI, idProjection, any(), any(), null)
        } returns null

        val id = reader.findId("+15551234567", 1234L)

        assertNull(id)
    }

    @Test
    fun `findId returns null on a missing READ_SMS grant (SecurityException), not a crash`() {
        every {
            contentResolver.query(Telephony.Sms.Inbox.CONTENT_URI, idProjection, any(), any(), null)
        } throws SecurityException("Permission Denial: reading com.android.providers.telephony")

        val id = reader.findId("+15551234567", 1234L)

        assertNull(id)
    }

    // --- readSince -------------------------------------------------------

    @Test
    fun `readSince maps cursor rows to SmsInboxEntry`() {
        val cursor = MatrixCursor(readSinceProjection).apply {
            addRow(arrayOf<Any>(11L, "+15551234567", 6_000L))
            addRow(arrayOf<Any>(12L, "+15559876543", 8_000L))
        }
        every {
            contentResolver.query(Telephony.Sms.Inbox.CONTENT_URI, readSinceProjection, any(), any(), any())
        } returns cursor

        val entries = reader.readSince(sinceId = 0L)

        assertEquals(2, entries.size)
        assertEquals(SmsInboxEntry(id = 11L, address = "+15551234567", timestampMillis = 6_000L), entries[0])
        assertEquals(SmsInboxEntry(id = 12L, address = "+15559876543", timestampMillis = 8_000L), entries[1])
    }

    @Test
    fun `a missing ADDRESS column maps to a null address`() {
        val cursor = MatrixCursor(arrayOf(Telephony.Sms._ID, Telephony.Sms.DATE)).apply {
            addRow(arrayOf<Any>(9L, 9_000L))
        }
        every {
            contentResolver.query(Telephony.Sms.Inbox.CONTENT_URI, readSinceProjection, any(), any(), any())
        } returns cursor

        val entries = reader.readSince(sinceId = 0L)

        assertNull(entries.single().address)
        assertEquals(9L, entries.single().id)
        assertEquals(9_000L, entries.single().timestampMillis)
    }

    @Test
    fun `a null cursor from the provider is an empty list`() {
        every {
            contentResolver.query(Telephony.Sms.Inbox.CONTENT_URI, readSinceProjection, any(), any(), any())
        } returns null

        val entries = reader.readSince(sinceId = 0L)

        assertTrue(entries.isEmpty())
    }

    @Test
    fun `queries _id greater-than ascending so a caller can page forward through a backlog`() {
        // Issue #1123's paging lesson applies to the Inbox reader too: ASC +
        // bounded pages, never a single DESC/newest-N read.
        val cursor = MatrixCursor(readSinceProjection)
        val selection = slot<String>()
        val selectionArgs = slot<Array<String>>()
        val sortOrder = slot<String>()
        every {
            contentResolver.query(
                Telephony.Sms.Inbox.CONTENT_URI,
                readSinceProjection,
                capture(selection),
                capture(selectionArgs),
                capture(sortOrder),
            )
        } returns cursor

        reader.readSince(sinceId = 100L, limit = 50)

        assertEquals("${Telephony.Sms._ID} > ?", selection.captured)
        assertEquals(listOf("100"), selectionArgs.captured.toList())
        assertEquals("${Telephony.Sms._ID} ASC LIMIT 50", sortOrder.captured)
    }

    @Test
    fun `readSince returns an empty list on a missing READ_SMS grant (SecurityException), not a crash`() {
        every {
            contentResolver.query(Telephony.Sms.Inbox.CONTENT_URI, readSinceProjection, any(), any(), any())
        } throws SecurityException("Permission Denial: reading com.android.providers.telephony")

        val entries = reader.readSince(sinceId = 0L)

        assertTrue(entries.isEmpty())
    }
}
