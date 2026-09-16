package com.mycorrhizal.crm.data.local

import java.io.File
import java.io.IOException
import org.junit.After
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/**
 * Issue #811: JVM unit coverage for [RoomCacheEncryption.isEncrypted]'s pure
 * header-check logic, plus (coverage-analysis follow-up) the crash-recovery
 * state machine in [RoomCacheEncryption.runReencryption] and the secure-erase
 * [RoomCacheEncryption.overwriteFile]. Unlike the two SQLCipher-touching steps
 * ([RoomCacheEncryption.ensureEncrypted]'s `exportToEncrypted`/
 * `rebuildFtsMirrors`, which need `libsqlcipher.so` — an Android-native
 * (Bionic) binary that cannot load even under Robolectric, see
 * `Migration13To14Test`'s class doc), everything tested here is plain
 * `java.io.File` control flow, so `runReencryption` takes those two steps as
 * injected lambdas and every branch is exercised with fakes. The instrumented
 * counterpart (`RoomCacheEncryptionTest` in `app/src/androidTest`) drives the
 * real SQLCipher calls end to end but, being a scripted happy-path test, never
 * reaches the failure branches below.
 *
 * `@RunWith(RobolectricTestRunner::class)` only so the production code's
 * `android.util.Log` calls (on every branch, including the happy path) don't
 * throw "not mocked" — Robolectric shadows `Log` as a no-op instead.
 *
 * The 16-byte plaintext magic a real SQLite file opens with is hardcoded below
 * rather than imported: [RoomCacheEncryption]'s `PLAINTEXT_MAGIC` is private,
 * and the assertion is against the literal wire format, not the source
 * constant that (if ever typoed) would make the test agree with the bug.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
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
        dir.listFiles()?.forEach { it.delete() }
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

    // --- overwriteFile: the secure-erase step, plain java.io/SecureRandom. ---

    @Test
    fun `overwriteFile replaces the content but keeps the file length`() {
        val original = "SQLite format 3 padding data".repeat(50).toByteArray(Charsets.ISO_8859_1)
        val file = fileWith(original)

        RoomCacheEncryption.overwriteFile(file)

        assertEquals(original.size.toLong(), file.length())
        assertFalse(
            "the overwrite must not reproduce the original bytes",
            original.contentEquals(file.readBytes()),
        )
    }

    @Test
    fun `overwriteFile on an empty file does not throw and leaves it empty`() {
        val file = fileWith(ByteArray(0))

        RoomCacheEncryption.overwriteFile(file)

        assertEquals(0L, file.length())
    }

    @Test
    fun `overwriteFile handles content larger than its internal buffer`() {
        // The internal buffer is 64KiB; exercise the multi-chunk loop.
        val original = ByteArray(200 * 1024) { it.toByte() }
        val file = fileWith(original)

        RoomCacheEncryption.overwriteFile(file)

        assertEquals(original.size.toLong(), file.length())
        assertFalse(original.contentEquals(file.readBytes()))
    }

    // --- runReencryption: the crash-recovery state machine (coverage-analysis
    // follow-up). The two SQLCipher-touching steps are injected as fakes; see
    // the class doc for why the real ones can't run here. ---

    private fun dbFileNamed(name: String, content: ByteArray): File {
        val file = File(dir, name)
        file.writeBytes(content)
        files += file
        return file
    }

    private fun encryptedSiblingOf(dbFile: File): File = File(dbFile.parentFile, dbFile.name + ".encrypted")

    @Test
    fun `runReencryption swaps in the exported content and rebuilds the FTS mirrors`() {
        val dbFile = dbFileNamed("happy.db", "plaintext-content".toByteArray())
        // A real sidecar the transition must clean up afterward.
        val wal = File(dir, "happy.db-wal").apply { writeBytes(byteArrayOf(1)) }
        files += wal
        var rebuildCalled = false

        RoomCacheEncryption.runReencryption(
            dbFile = dbFile,
            export = { encrypted -> encrypted.writeBytes("encrypted-content".toByteArray()) },
            rebuildFts = { rebuildCalled = true },
        )

        assertEquals("encrypted-content", dbFile.readText())
        assertFalse("the temp .encrypted file must not linger", encryptedSiblingOf(dbFile).exists())
        assertFalse("sidecars must be cleaned up on a successful swap", wal.exists())
        assertTrue("the FTS rebuild step must run after a successful swap", rebuildCalled)
    }

    @Test
    fun `runReencryption leaves the plaintext file untouched when export fails`() {
        val original = "plaintext-content".toByteArray()
        val dbFile = dbFileNamed("export-fails.db", original)
        var rebuildCalled = false

        try {
            RoomCacheEncryption.runReencryption(
                dbFile = dbFile,
                export = { throw IOException("disk full") },
                rebuildFts = { rebuildCalled = true },
            )
            fail("export failing with no recoverable .encrypted file must rethrow")
        } catch (expected: IllegalStateException) {
            assertEquals("disk full", expected.cause?.message)
        }

        assertArrayEquals(
            "the plaintext must survive an export failure untouched",
            original,
            dbFile.readBytes(),
        )
        assertFalse(encryptedSiblingOf(dbFile).exists())
        assertFalse("the FTS rebuild must not run when the transition failed outright", rebuildCalled)
    }

    @Test
    fun `runReencryption recovers a failed swap by completing it when the export already succeeded`() {
        // An empty directory in place of dbFile makes overwriteFile's
        // FileOutputStream open fail ("Is a directory") *after* a successful
        // export, simulating the documented "secure-erase IO failure can
        // leave the plaintext corrupt mid-overwrite" case. The recovery path
        // deletes the (empty) directory and completes the swap itself.
        val dbFile = File(dir, "swap-fails.db").apply { assertTrue(mkdir()) }
        files += dbFile

        RoomCacheEncryption.runReencryption(
            dbFile = dbFile,
            export = { encrypted -> encrypted.writeBytes("encrypted-content".toByteArray()) },
            rebuildFts = {},
        )

        assertTrue("the recovery swap must leave a regular file, not the directory", dbFile.isFile)
        assertEquals("encrypted-content", dbFile.readText())
        assertFalse(encryptedSiblingOf(dbFile).exists())
    }

    @Test
    fun `runReencryption rethrows and cleans up the temp file when the swap recovery also fails`() {
        // A non-empty directory in place of dbFile: overwriteFile still fails
        // the same way, but this time the recovery's own dbFile.delete() also
        // fails (non-empty directory), so the recovery attempt fails too.
        val dbFile = File(dir, "swap-and-recovery-fail.db").apply { assertTrue(mkdir()) }
        files += dbFile
        File(dbFile, "not-empty").writeBytes(byteArrayOf(1))

        try {
            RoomCacheEncryption.runReencryption(
                dbFile = dbFile,
                export = { encrypted -> encrypted.writeBytes("encrypted-content".toByteArray()) },
                rebuildFts = {},
            )
            fail("a swap failure whose recovery also fails must rethrow")
        } catch (expected: IllegalStateException) {
            // The original (pre-recovery) failure is preserved as the cause.
            assertTrue(expected.cause is IOException)
        }

        assertFalse(
            "the temp .encrypted file must be cleaned up, not left behind, when recovery fails",
            encryptedSiblingOf(dbFile).exists(),
        )
        // The directory nobody could delete is exactly as it was — no data lost.
        assertTrue(dbFile.isDirectory)
        assertTrue(File(dbFile, "not-empty").exists())
    }

    @Test
    fun `runReencryption treats a post-swap step failure as non-fatal (bug fix)`() {
        // Regression test for the ordering bug: `swapped` used to flip only
        // after the sidecar-cleanup step returned, so a failure *in* that step
        // could never be attributed to "swap already succeeded" -- see the
        // production KDoc on runReencryption for the full story. Hand-verified:
        // reverting the fix (moving `swapped = true` back below `postSwap(...)`)
        // makes this test fail with an unexpected IllegalStateException instead
        // of completing normally.
        val dbFile = dbFileNamed("post-swap-fails.db", "plaintext-content".toByteArray())
        var rebuildCalled = false

        RoomCacheEncryption.runReencryption(
            dbFile = dbFile,
            export = { encrypted -> encrypted.writeBytes("encrypted-content".toByteArray()) },
            rebuildFts = { rebuildCalled = true },
            postSwap = { throw IOException("sidecar cleanup failed") },
        )

        // The swap already completed: dbFile must hold the encrypted content,
        // and the transition must be reported as having succeeded (no thrown
        // exception) rather than "cache left untouched".
        assertEquals("encrypted-content", dbFile.readText())
        assertTrue("the FTS rebuild must still run after a non-fatal post-swap failure", rebuildCalled)
    }

    @Test
    fun `runReencryption swallows an FTS rebuild failure without rethrowing`() {
        val dbFile = dbFileNamed("fts-fails.db", "plaintext-content".toByteArray())

        RoomCacheEncryption.runReencryption(
            dbFile = dbFile,
            export = { encrypted -> encrypted.writeBytes("encrypted-content".toByteArray()) },
            rebuildFts = { throw IllegalStateException("FTS index corrupt") },
        )

        // The swap itself still succeeded; only the (best-effort) search index
        // rebuild failed, and that must degrade search, not fail the boot.
        assertEquals("encrypted-content", dbFile.readText())
    }
}
