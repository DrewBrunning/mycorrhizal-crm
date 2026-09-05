package com.mycorrhizal.crm.data.local

import java.io.File
import org.junit.After
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * Issue #811: JVM unit coverage for [RoomCacheEncryption.isEncrypted]'s pure
 * header-check logic. Unlike the SQLCipher-touching methods in the same object
 * ([RoomCacheEncryption.ensureEncrypted]/[RoomCacheEncryption.reencryptInPlace]
 * need `System.loadLibrary("sqlcipher")` and are instrumented-only),
 * `isEncrypted` is plain `java.io.File` I/O, so every branch is exercised here
 * directly with temp files — no Robolectric, no emulator. The instrumented
 * counterpart (`RoomCacheEncryptionTest` in `app/src/androidTest`) only ever
 * produces the two outcomes its end-to-end flows happen to reach.
 *
 * The 16-byte plaintext magic a real SQLite file opens with is hardcoded below
 * rather than imported: [RoomCacheEncryption]'s `PLAINTEXT_MAGIC` is private,
 * and the assertion is against the literal wire format, not the source
 * constant that (if ever typoed) would make the test agree with the bug.
 */
class RoomCacheEncryptionTest {

    private lateinit var dir: File
    private val files = mutableListOf<File>()

    @Before
    fun setUp() {
        dir = File(
            System.getProperty("java.io.tmpdir"),
            "room-cache-encryption-test-${System.nanoTime()}",
        )
        assertTrue(dir.mkdirs())
    }

    @After
    fun tearDown() {
        files.forEach { it.delete() }
        dir.delete()
    }

    private fun fileWith(content: ByteArray): File {
        val file = File(dir, "db-${files.size}.db")
        file.writeBytes(content)
        files += file
        return file
    }

    private val plaintextHeader = "SQLite format 3\u0000"

    @Test
    fun `a missing file is treated as encrypted`() {
        // Fresh installs have no database yet; there is nothing to read, so the
        // file must not be mistaken for plaintext waiting to be transitioned.
        assertTrue(RoomCacheEncryption.isEncrypted(File(dir, "does-not-exist.db")))
    }

    @Test
    fun `an empty file is treated as encrypted`() {
        assertTrue(RoomCacheEncryption.isEncrypted(fileWith(ByteArray(0))))
    }

    @Test
    fun `a file shorter than the 16-byte magic header is treated as encrypted`() {
        assertTrue(
            RoomCacheEncryption.isEncrypted(
                fileWith(plaintextHeader.dropLast(1).toByteArray(Charsets.ISO_8859_1)),
            ),
        )
    }

    @Test
    fun `a real plaintext SQLite header is not encrypted`() {
        assertFalse(
            RoomCacheEncryption.isEncrypted(
                fileWith(plaintextHeader.toByteArray(Charsets.ISO_8859_1)),
            ),
        )
    }

    @Test
    fun `garbage bytes of the same length as the magic are treated as encrypted`() {
        val garbage = "SQLite format 3X".toByteArray(Charsets.ISO_8859_1)
        assertTrue(RoomCacheEncryption.isEncrypted(fileWith(garbage)))
    }

    @Test
    fun `a null byte in place of the magic's final byte is treated as encrypted`() {
        // Only the header's first 15 bytes match; the terminator differs. A
        // SQLCipher file's salt bytes could collide with the prefix, so the
        // comparison must be over the whole 16 bytes.
        assertTrue(
            RoomCacheEncryption.isEncrypted(
                fileWith("SQLite format 3\u0001".toByteArray(Charsets.ISO_8859_1)),
            ),
        )
    }
}
