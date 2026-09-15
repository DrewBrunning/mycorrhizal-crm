package com.mycorrhizal.crm.data.session

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Guard test for the OIDC pending-request storage backend (issue #965, same
 * rationale as [EncryptedDeviceGrantStorageGuardTest] / [EncryptedTokenStorageGuardTest]).
 * The PKCE code verifier is a short-lived credential: possession of it plus a
 * stolen deep-link code would redeem a session. It must live in
 * [EncryptedSharedPreferences] behind a Keystore [MasterKey], never in a plain
 * `SharedPreferences`. [EncryptedOidcPendingRequestStore] cannot be exercised
 * directly on the JVM (no Keystore under Robolectric), so this asserts on the
 * source and the DI wiring.
 */
class EncryptedOidcPendingRequestStorageGuardTest {

    private val storageSource =
        File("src/main/kotlin/com/mycorrhizal/crm/data/session/OidcPendingRequestStore.kt").readText()

    private val diSource =
        File("src/main/kotlin/com/mycorrhizal/crm/data/di/SessionStorageModule.kt").readText()

    @Test
    fun `the pending request is stored via EncryptedSharedPreferences, not plain SharedPreferences`() {
        assertTrue(
            "EncryptedOidcPendingRequestStore must use EncryptedSharedPreferences.create",
            storageSource.contains("EncryptedSharedPreferences.create"),
        )
        assertFalse(
            "EncryptedOidcPendingRequestStore must not fall back to a plain getSharedPreferences",
            storageSource.contains("getSharedPreferences"),
        )
    }

    @Test
    fun `the encryption key is a Keystore-backed MasterKey with AES256_GCM`() {
        assertTrue(storageSource.contains("MasterKey.Builder"))
        assertTrue(storageSource.contains("MasterKey.KeyScheme.AES256_GCM"))
    }

    @Test
    fun `pref keys and values use the recommended encryption schemes`() {
        assertTrue(storageSource.contains("PrefKeyEncryptionScheme.AES256_SIV"))
        assertTrue(storageSource.contains("PrefValueEncryptionScheme.AES256_GCM"))
    }

    @Test
    fun `the DI graph wires the encrypted store as the OidcPendingRequestStore implementation`() {
        assertTrue(
            "provideOidcPendingRequestStore must return an EncryptedOidcPendingRequestStore",
            diSource.contains("EncryptedOidcPendingRequestStore(context)"),
        )
    }
}
