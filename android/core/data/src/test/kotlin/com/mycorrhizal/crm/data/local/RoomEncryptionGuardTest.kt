package com.mycorrhizal.crm.data.local

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Guard test for the Room-mirror encryption stack (issue #385, MASVS-L1
 * STORAGE-1/CRYPTO-1). The SQLCipher passphrase and the encryption wiring live
 * behind Keystore-backed primitives that cannot be exercised directly on the
 * JVM (see [EncryptedTokenStorageGuardTest] for the same pattern), so this
 * asserts on the source: it fails if the passphrase store is downgraded to a
 * plain `SharedPreferences`, if the SQLCipher open-helper factory is dropped
 * from the DI wiring, if the plaintext→encrypted transition stops being run
 * before the database opens, or if the session no longer wipes cached data.
 */
class RoomEncryptionGuardTest {

    private val passphraseStoreSource =
        File("src/main/kotlin/com/mycorrhizal/crm/data/local/RoomPassphraseStore.kt").readText()

    private val transitionSource =
        File("src/main/kotlin/com/mycorrhizal/crm/data/local/RoomCacheEncryption.kt").readText()

    private val dataModuleSource =
        File("src/main/kotlin/com/mycorrhizal/crm/data/di/DataModule.kt").readText()

    private val cleanerSource =
        File("src/main/kotlin/com/mycorrhizal/crm/data/local/LocalDataCleaner.kt").readText()

    @Test
    fun `the SQLCipher passphrase is stored via EncryptedSharedPreferences, not a plain prefs file`() {
        assertTrue(
            "RoomPassphraseStore must use EncryptedSharedPreferences.create",
            passphraseStoreSource.contains("EncryptedSharedPreferences.create"),
        )
        assertFalse(
            "RoomPassphraseStore must not fall back to a plain getSharedPreferences",
            passphraseStoreSource.contains("getSharedPreferences"),
        )
    }

    @Test
    fun `the passphrase is a fresh 32-byte SecureRandom value, generated on first use`() {
        assertTrue(
            "RoomPassphraseStore must generate the passphrase from SecureRandom (issue #812 moved the bytes into randomHexPassphrase)",
            passphraseStoreSource.contains("random.nextBytes(bytes)"),
        )
        assertTrue(
            "The passphrase must be 32 bytes by default (256-bit key material)",
            passphraseStoreSource.contains("byteCount: Int = 32"),
        )
    }

    @Test
    fun `the database is opened through the SQLCipher open-helper factory`() {
        assertTrue(
            "provideDatabase must call ensureEncrypted before Room opens",
            dataModuleSource.contains("RoomCacheEncryption.ensureEncrypted"),
        )
        assertTrue(
            "provideDatabase must set the SQLCipher SupportOpenHelperFactory",
            dataModuleSource.contains("SupportOpenHelperFactory"),
        )
    }

    @Test
    fun `the plaintext-to-encrypted transition preserves the whole database`() {
        assertTrue(
            "The transition must export every table (incl. FTS + the outbox) via sqlcipher_export",
            transitionSource.contains("sqlcipher_export"),
        )
        assertTrue(
            "The transition must overwrite the plaintext file before deletion",
            transitionSource.contains("overwriteFile"),
        )
    }

    @Test
    fun `ending a session wipes the Room mirror and cached images`() {
        assertTrue(
            "LocalDataCleaner must clear the Room database tables",
            cleanerSource.contains("clearAllTables"),
        )
        assertTrue(
            "LocalDataCleaner must delete the cached image files",
            cleanerSource.contains("deleteRecursively"),
        )
    }
}
