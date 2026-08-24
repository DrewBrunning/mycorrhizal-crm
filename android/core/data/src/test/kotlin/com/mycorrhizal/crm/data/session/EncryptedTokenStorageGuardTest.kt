package com.mycorrhizal.crm.data.session

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Guard test for the session-token storage backend (issue #367, MASVS-L1
 * STORAGE-1 / CRYPTO-1). The JWT must live in [EncryptedSharedPreferences]
 * behind a Keystore [MasterKey], never in a plain `SharedPreferences`.
 *
 * [EncryptedTokenStorage] cannot be exercised directly on the JVM (the Android
 * Keystore is unavailable under Robolectric), so the test asserts on the source
 * itself: it fails if the encrypted backend is swapped for a plain
 * `getSharedPreferences`, if the Keystore master key is dropped, or if the DI
 * binding stops wiring [EncryptedTokenStorage] as the [TokenStorage]
 * implementation. Files resolve relative to the module working directory, the
 * same pattern as `core/ui`'s `LocalesConsistencyTest`.
 */
class EncryptedTokenStorageGuardTest {

    private val tokenStorageSource =
        File("src/main/kotlin/com/mycorrhizal/crm/data/session/EncryptedTokenStorage.kt").readText()

    private val diSource =
        File("src/main/kotlin/com/mycorrhizal/crm/data/di/SessionStorageModule.kt").readText()

    @Test
    fun `the token is stored via EncryptedSharedPreferences, not plain SharedPreferences`() {
        assertTrue(
            "EncryptedTokenStorage must use EncryptedSharedPreferences.create",
            tokenStorageSource.contains("EncryptedSharedPreferences.create"),
        )
        assertFalse(
            "EncryptedTokenStorage must not fall back to a plain getSharedPreferences",
            tokenStorageSource.contains("getSharedPreferences"),
        )
    }

    @Test
    fun `the encryption key is a Keystore-backed MasterKey with AES256_GCM`() {
        assertTrue(
            "EncryptedTokenStorage must build a Keystore MasterKey",
            tokenStorageSource.contains("MasterKey.Builder"),
        )
        assertTrue(
            "The master key scheme must be AES256_GCM",
            tokenStorageSource.contains("MasterKey.KeyScheme.AES256_GCM"),
        )
    }

    @Test
    fun `pref keys and values use the recommended encryption schemes`() {
        assertTrue(
            "Pref keys must use AES256_SIV",
            tokenStorageSource.contains("PrefKeyEncryptionScheme.AES256_SIV"),
        )
        assertTrue(
            "Pref values must use AES256_GCM",
            tokenStorageSource.contains("PrefValueEncryptionScheme.AES256_GCM"),
        )
    }

    @Test
    fun `the DI graph wires EncryptedTokenStorage as the TokenStorage implementation`() {
        assertTrue(
            "provideTokenStorage must return an EncryptedTokenStorage",
            diSource.contains("EncryptedTokenStorage"),
        )
    }
}
