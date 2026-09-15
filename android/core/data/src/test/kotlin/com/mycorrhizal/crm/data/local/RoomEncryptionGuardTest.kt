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
 * plain `SharedPreferences`, if the passphrase write stops being synchronous
 * (issue #998), if the SQLCipher open-helper factory is dropped from the DI
 * wiring, if the DB-open recovery backstop is dropped, if the
 * plaintext→encrypted transition stops being run before the database opens,
 * or if the session no longer wipes cached data.
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
    fun `a freshly generated passphrase is committed synchronously and verified, not fire-and-forget (issue 998)`() {
        assertTrue(
            "getOrCreate must persist with commit() so the write is durable before the passphrase is used",
            passphraseStoreSource.contains(".putString(KEY_PASSPHRASE, passphrase).commit()"),
        )
        assertFalse(
            "getOrCreate must not persist the freshly generated passphrase with the async, " +
                "fire-and-forget apply() (clear()'s apply() is fine: it's only ever used " +
                "alongside deleting the DB file outright, not before using the value)",
            passphraseStoreSource.contains(".putString(KEY_PASSPHRASE, passphrase).apply()"),
        )
        assertTrue(
            "getOrCreate must read the value back to confirm it actually landed",
            passphraseStoreSource.contains("prefs.getString(KEY_PASSPHRASE, null) == passphrase"),
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
    fun `an undecryptable database is recovered instead of boot-looping (issue 998)`() {
        assertTrue(
            "provideDatabase must open through RoomDatabaseRecovery so a wrong/lost passphrase " +
                "wipes and rebuilds the mirror instead of crashing forever",
            dataModuleSource.contains("RoomDatabaseRecovery.openOrRebuild"),
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
