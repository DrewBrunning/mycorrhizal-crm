package com.mycorrhizal.crm.data.local

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.security.SecureRandom

/**
 * Unit tests for [randomHexPassphrase] (issue #812). The rest of
 * [RoomPassphraseStore] — the constructor, [RoomPassphraseStore.getOrCreate]
 * and [RoomPassphraseStore.clear] — sits behind EncryptedSharedPreferences +
 * a Keystore MasterKey that does not exist in a plain JVM sandbox, so it stays
 * instrumentation-only (covered by `android/app`'s androidTest per issue #385).
 */
class RoomPassphraseStoreTest {

    @Test
    fun `default passphrase hex-encodes 32 bytes to 64 characters`() {
        assertEquals(64, randomHexPassphrase().length)
    }

    @Test
    fun `output length is always byteCount times two`() {
        for (byteCount in intArrayOf(0, 1, 8, 32)) {
            assertEquals(byteCount * 2, randomHexPassphrase(byteCount = byteCount).length)
        }
    }

    @Test
    fun `every character is a valid lowercase hex digit`() {
        val hex = randomHexPassphrase()
        assertTrue("expected length 64 but was ${hex.length}", hex.length == 64)
        assertTrue(
            "characters must be lowercase hex digits, got $hex",
            hex.all { it in '0'..'9' || it in 'a'..'f' },
        )
    }

    @Test
    fun `independently seeded instances produce different output`() {
        // Sanity check that randomness actually flows through rather than a
        // hardcoded value being returned.
        val first = randomHexPassphrase(random = SecureRandom(byteArrayOf(1)))
        val second = randomHexPassphrase(random = SecureRandom(byteArrayOf(2)))
        assertNotEquals(first, second)
    }

    @Test
    fun `known bytes encode to exact lowercase hex with no separators or prefix`() {
        // Pins the encoding the doc comment relies on for splicing into a
        // `KEY '<hex>'` SQL literal: lowercase, 2 chars per byte, no `0x`
        // prefix, no separators.
        val deterministic = FixedBytesSecureRandom(
            byteArrayOf(0x00, 0x0f, 0x10, 0xab.toByte(), 0xff.toByte(), 0x5a),
        )
        assertEquals(
            "000f10abff5a",
            randomHexPassphrase(random = deterministic, byteCount = 6),
        )
    }

    private class FixedBytesSecureRandom(private val fixed: ByteArray) : SecureRandom() {
        override fun nextBytes(bytes: ByteArray) {
            fixed.copyInto(bytes)
        }
    }
}
