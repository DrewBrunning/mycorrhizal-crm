package com.mycorrhizal.crm.feature.contacts

import android.content.ContentResolver
import android.content.Context
import android.content.ContextWrapper
import android.content.Intent
import android.database.MatrixCursor
import android.net.Uri
import android.provider.OpenableColumns
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.model.network.ContactAttachment
import io.mockk.every
import io.mockk.mockk
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import java.io.File

/**
 * AttachmentsScreen.kt (63 missed lines, the highest-missed Android file in
 * the coverage-analysis triage) has real non-Composable logic worth testing
 * in isolation: the upload size-limit/empty-file decision, `queryAttachmentMeta`'s
 * cursor handling, the byte-formatting behind the row subtitle, and the
 * swallow-on-failure around opening a downloaded file. Each was pulled out to
 * an `internal` pure (or Robolectric-testable) function for exactly that
 * reason — see each function's doc comment in AttachmentsScreen.kt. The
 * Composable rendering itself stays covered by [AttachmentsScreenTest]
 * (Compose UI test), not here.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
class AttachmentsScreenLogicTest {

    // --- exceedsAttachmentSizeLimit: the shared metadata-probe / byte-length-backstop predicate ---

    @Test
    fun `a size under the cap does not exceed the limit`() {
        assertFalse(exceedsAttachmentSizeLimit(size = 1024L, maxSizeBytes = 2048L))
    }

    @Test
    fun `a size exactly at the cap does not exceed the limit`() {
        assertFalse(exceedsAttachmentSizeLimit(size = 2048L, maxSizeBytes = 2048L))
    }

    @Test
    fun `a size over the cap exceeds the limit`() {
        assertTrue(exceedsAttachmentSizeLimit(size = 2049L, maxSizeBytes = 2048L))
    }

    @Test
    fun `a null size (provider reports none) never exceeds the limit here`() {
        // The caller relies on the post-read byte-length call as the backstop
        // for this case; a null size must not itself be treated as a rejection.
        assertFalse(exceedsAttachmentSizeLimit(size = null, maxSizeBytes = 2048L))
    }

    // --- resolveAttachmentUpload: filename/MIME-type fallback derivation ---

    @Test
    fun `resolveAttachmentUpload uses the probed name and mime type when present`() {
        val meta = AttachmentPick(name = "scan.pdf", mimeType = "application/pdf", size = 10L)
        val bytes = byteArrayOf(1, 2, 3)

        val upload = resolveAttachmentUpload(meta, bytes)

        assertEquals("scan.pdf", upload.fileName)
        assertEquals("application/pdf", upload.mimeType)
        assertTrue(bytes.contentEquals(upload.bytes))
    }

    @Test
    fun `resolveAttachmentUpload falls back to a generic name when the provider reports none`() {
        val meta = AttachmentPick(name = null, mimeType = "application/pdf", size = 10L)

        val upload = resolveAttachmentUpload(meta, byteArrayOf())

        assertEquals("attachment", upload.fileName)
    }

    @Test
    fun `resolveAttachmentUpload falls back to a generic name when the provider reports a blank one`() {
        val meta = AttachmentPick(name = "   ", mimeType = "application/pdf", size = 10L)

        val upload = resolveAttachmentUpload(meta, byteArrayOf())

        assertEquals("attachment", upload.fileName)
    }

    @Test
    fun `resolveAttachmentUpload falls back to a generic mime type when the provider reports none`() {
        val meta = AttachmentPick(name = "scan.pdf", mimeType = null, size = 10L)

        val upload = resolveAttachmentUpload(meta, byteArrayOf())

        assertEquals("application/octet-stream", upload.mimeType)
    }

    // --- attachmentSizeText: the byte/KB/MB decision behind the row subtitle ---

    @Test
    fun `attachmentSizeText is null for a zero size`() {
        assertNull(attachmentSizeText(0L))
    }

    @Test
    fun `attachmentSizeText is null for a negative size`() {
        assertNull(attachmentSizeText(-1L))
    }

    @Test
    fun `attachmentSizeText formats a positive size`() {
        assertEquals("1.0 KB", attachmentSizeText(1024L))
    }

    // --- queryAttachmentMeta: cursor-column-missing handling ---

    private val fullProjection = arrayOf(OpenableColumns.DISPLAY_NAME, OpenableColumns.SIZE)
    private val resolver = mockk<ContentResolver>()
    private val uri: Uri = Uri.parse("content://com.example.provider/document/1")

    @Test
    fun `queryAttachmentMeta reads the name and size when both columns are present`() {
        val cursor = MatrixCursor(fullProjection).apply {
            addRow(arrayOf<Any>("scan.pdf", 2048L))
        }
        every { resolver.query(uri, null, null, null, null) } returns cursor
        every { resolver.getType(uri) } returns "application/pdf"

        val meta = queryAttachmentMeta(resolver, uri)

        assertEquals("scan.pdf", meta.name)
        assertEquals(2048L, meta.size)
        assertEquals("application/pdf", meta.mimeType)
    }

    @Test
    fun `queryAttachmentMeta tolerates a cursor missing the DISPLAY_NAME column`() {
        val projectionWithoutName = arrayOf(OpenableColumns.SIZE)
        val cursor = MatrixCursor(projectionWithoutName).apply {
            addRow(arrayOf<Any>(2048L))
        }
        every { resolver.query(uri, null, null, null, null) } returns cursor
        every { resolver.getType(uri) } returns "application/pdf"

        val meta = queryAttachmentMeta(resolver, uri)

        assertNull(meta.name)
        assertEquals(2048L, meta.size)
    }

    @Test
    fun `queryAttachmentMeta tolerates a cursor missing the SIZE column`() {
        val projectionWithoutSize = arrayOf(OpenableColumns.DISPLAY_NAME)
        val cursor = MatrixCursor(projectionWithoutSize).apply {
            addRow(arrayOf<Any>("scan.pdf"))
        }
        every { resolver.query(uri, null, null, null, null) } returns cursor
        every { resolver.getType(uri) } returns "application/pdf"

        val meta = queryAttachmentMeta(resolver, uri)

        assertEquals("scan.pdf", meta.name)
        assertNull(meta.size)
    }

    @Test
    fun `queryAttachmentMeta tolerates a cursor missing both columns`() {
        val cursor = MatrixCursor(arrayOf("_id")).apply { addRow(arrayOf<Any>(1L)) }
        every { resolver.query(uri, null, null, null, null) } returns cursor
        every { resolver.getType(uri) } returns null

        val meta = queryAttachmentMeta(resolver, uri)

        assertNull(meta.name)
        assertNull(meta.size)
        assertNull(meta.mimeType)
    }

    @Test
    fun `queryAttachmentMeta tolerates a null column value`() {
        // A provider can report the DISPLAY_NAME column but leave the row's
        // value null; that must read back as a null name, not crash reading it.
        val cursor = MatrixCursor(fullProjection).apply {
            addRow(arrayOf<Any?>(null, 2048L))
        }
        every { resolver.query(uri, null, null, null, null) } returns cursor
        every { resolver.getType(uri) } returns "application/pdf"

        val meta = queryAttachmentMeta(resolver, uri)

        assertNull(meta.name)
        assertEquals(2048L, meta.size)
    }

    @Test
    fun `queryAttachmentMeta tolerates an empty cursor (no row)`() {
        val cursor = MatrixCursor(fullProjection)
        every { resolver.query(uri, null, null, null, null) } returns cursor
        every { resolver.getType(uri) } returns "application/pdf"

        val meta = queryAttachmentMeta(resolver, uri)

        assertNull(meta.name)
        assertNull(meta.size)
    }

    @Test
    fun `queryAttachmentMeta tolerates a null cursor from the provider`() {
        every { resolver.query(uri, null, null, null, null) } returns null
        every { resolver.getType(uri) } returns "application/pdf"

        val meta = queryAttachmentMeta(resolver, uri)

        assertNull(meta.name)
        assertNull(meta.size)
        assertEquals("application/pdf", meta.mimeType)
    }

    // --- openDownloadedAttachment: the swallow-on-failure around launching the viewer ---

    @Test
    fun `openDownloadedAttachment swallows a failure launching the viewer`() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        val throwingContext = object : ContextWrapper(context) {
            override fun startActivity(intent: Intent) {
                throw android.content.ActivityNotFoundException("no activity can view this attachment")
            }
        }
        val attachment = ContactAttachment(
            id = 7,
            originalName = "scan.pdf",
            contentType = "application/pdf",
            sizeBytes = 3,
        )

        // Must not throw: a provider with no registered viewer is expected,
        // not exceptional, and must never crash the screen.
        openDownloadedAttachment(throwingContext, attachment, byteArrayOf(1, 2, 3))

        // The download still lands in the cache even though no viewer opened it.
        val written = File(File(context.cacheDir, "attachments"), "scan.pdf")
        assertTrue(written.exists())
        assertEquals(3, written.length())
    }

    @Test
    fun `openDownloadedAttachment sanitizes an unsafe original name before writing to disk`() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        val throwingContext = object : ContextWrapper(context) {
            override fun startActivity(intent: Intent) {
                throw android.content.ActivityNotFoundException("no activity can view this attachment")
            }
        }
        val attachment = ContactAttachment(
            id = 9,
            originalName = "../../etc/passwd",
            contentType = "text/plain",
            sizeBytes = 1,
        )

        openDownloadedAttachment(throwingContext, attachment, byteArrayOf(1))

        val dir = File(context.cacheDir, "attachments")
        // The path-traversal characters must be neutralized: nothing escapes
        // the attachments cache directory.
        assertFalse(File(dir.parentFile, "etc/passwd").exists())
        assertTrue(dir.listFiles()?.any { it.name == ".._.._etc_passwd" } == true)
    }
}
