package com.mycorrhizal.crm.data.local

import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File
import java.util.Base64

/**
 * Issue #1262: Keystore-wrapped local-server secrets. The generation/codec
 * logic is exercised with a fake cipher; the real Keystore cipher is pinned
 * separately by the guard test.
 */
class LocalServerSecretStoreTest {

    @get:Rule
    val temp = TemporaryFolder()

    private val purposesUsed = mutableListOf<LocalServerSecretPurpose>()

    /** A reversible "cipher" that also proves per-purpose key separation by tagging. */
    private class TaggingCipher(private val tag: Byte) : LocalServerSecretCipher {
        override fun wrap(plaintext: ByteArray): ByteArray = byteArrayOf(tag) + plaintext.reversedArray()

        override fun unwrap(blob: ByteArray): ByteArray {
            assertEquals(tag, blob.first())
            return blob.copyOfRange(1, blob.size).reversedArray()
        }
    }

    private fun provider(): LocalServerSecretCipherProvider = LocalServerSecretCipherProvider { purpose ->
        purposesUsed += purpose
        when (purpose) {
            LocalServerSecretPurpose.JWT_SECRET -> TaggingCipher(1)
            LocalServerSecretPurpose.ATREST_MASTER_KEY -> TaggingCipher(2)
        }
    }

    private fun newStore(name: String = "keys.bin"): LocalServerSecretStore =
        LocalServerSecretStore(File(temp.root, name), provider())

    @Test
    fun `first run generates two 32-byte secrets and persists them`() {
        val file = File(temp.root, "keys.bin")
        val store = LocalServerSecretStore(file, provider())

        val secrets = store.loadOrCreate()

        assertTrue(file.exists())
        val lines = file.readText().trim().split('\n')
        assertEquals(LocalServerSecretStore.FORMAT_HEADER, lines[0])
        assertEquals(3, lines.size)
        // Both purposes had their own cipher invoked.
        assertEquals(
            setOf(LocalServerSecretPurpose.JWT_SECRET, LocalServerSecretPurpose.ATREST_MASTER_KEY),
            purposesUsed.toSet(),
        )
        // Round-trips through the recorded encodings.
        assertEquals(32, Base64.getUrlDecoder().decode(secrets.jwtSecret).size)
        assertEquals(32, Base64.getDecoder().decode(secrets.dataEncryptionKey).size)
    }

    @Test
    fun `a second load returns the same secrets without regenerating`() {
        val first = newStore().loadOrCreate()
        val second = newStore().loadOrCreate()

        assertEquals(first, second)
    }

    @Test
    fun `two independent stores generate different secrets`() {
        val a = newStore("a.bin").loadOrCreate()
        val b = newStore("b.bin").loadOrCreate()
        assertNotEquals(a, b)
    }

    @Test
    fun `a malformed file is refused rather than regenerated`() {
        val file = temp.newFile("keys.bin").apply { writeText("garbage\n") }
        val store = LocalServerSecretStore(file, provider())

        val failure = runCatching { store.loadOrCreate() }.exceptionOrNull()
        assertTrue(failure is IllegalArgumentException)
        // The file is left untouched: a silent regeneration would orphan the
        // existing database's encrypted fields.
        assertEquals("garbage\n", file.readText())
    }

    @Test
    fun `delete removes the wrapped keys`() {
        val file = File(temp.root, "delete.bin")
        val store = LocalServerSecretStore(file, provider())
        store.loadOrCreate()
        assertTrue(file.exists())

        store.delete()

        assertFalse(file.exists())
    }

    @Test
    fun `the wrapped blobs differ from the plaintext secrets`() {
        val file = File(temp.root, "wrapped.bin")
        val store = LocalServerSecretStore(file, provider())
        val secrets = store.loadOrCreate()

        val raw = file.readText()
        val jwt = Base64.getUrlDecoder().decode(secrets.jwtSecret)
        // The on-disk record is the cipher output, not the decoded secret.
        assertFalse(raw.contains(Base64.getEncoder().encodeToString(jwt)))
    }

    @Test
    fun `cipher round trips the raw bytes`() {
        val cipher = TaggingCipher(7)
        val plain = ByteArray(32) { it.toByte() }
        val blob = cipher.wrap(plain)
        assertArrayEquals(plain, cipher.unwrap(blob))
    }
}
