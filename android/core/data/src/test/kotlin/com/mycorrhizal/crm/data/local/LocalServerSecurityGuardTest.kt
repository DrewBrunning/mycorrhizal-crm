package com.mycorrhizal.crm.data.local

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Issue #1262 / ADR 0028 Decision 2 and 4. The real Keystore cipher cannot be
 * exercised on the JVM (no Keystore under Robolectric), so this asserts the
 * security-relevant source properties the host depends on, the same way the
 * token/device-grant store guard tests do.
 */
class LocalServerSecurityGuardTest {

    // The Keystore cipher lives in its own file (outside the coverage model);
    // the store/codec in LocalServerSecrets.kt. Both are asserted together so a
    // refactor cannot quietly drop the AndroidKeyStore + per-purpose-key
    // properties.
    private val secretsSource = listOf(
        "src/main/kotlin/com/mycorrhizal/crm/data/local/LocalServerSecrets.kt",
        "src/main/kotlin/com/mycorrhizal/crm/data/local/KeystoreLocalServerSecretCipher.kt",
    ).joinToString("\n") { File(it).readText() }
    private val hostSource =
        File("src/main/kotlin/com/mycorrhizal/crm/data/local/LocalServerHost.kt").readText()
    private val cleanerSource =
        File("src/main/kotlin/com/mycorrhizal/crm/data/local/LocalDataCleaner.kt").readText()
    private val moduleSource =
        File("src/main/kotlin/com/mycorrhizal/crm/data/di/LocalServerModule.kt").readText()

    @Test
    fun `the secrets are wrapped by a per-purpose Android Keystore AES-GCM key`() {
        assertTrue(secretsSource.contains("AndroidKeyStore"))
        assertTrue(secretsSource.contains("AES/GCM/NoPadding"))
        assertTrue(secretsSource.contains("setKeySize(256)"))
        // One dedicated key per purpose (MASVS CRYPTO).
        assertTrue(secretsSource.contains("mycorrhizal.local_server.jwt"))
        assertTrue(secretsSource.contains("mycorrhizal.local_server.atrest"))
    }

    @Test
    fun `the embedded store lives under filesDir, never the wiped cache`() {
        assertTrue(hostSource.contains("\"local-server\""))
        assertTrue(moduleSource.contains("context.filesDir"))
        // The Room-mirror cleaner wipes cacheDir; it must not know about the
        // local server's store, or logout/switch would destroy the only copy.
        assertTrue(cleanerSource.contains("context.cacheDir.deleteRecursively()"))
        assertFalse(cleanerSource.contains("local-server"))
    }

    @Test
    fun `the host clears the child environment`() {
        assertTrue(hostSource.contains("environment = mapOf("))
        // Only non-secret values (HOME/TMPDIR) are passed; secrets go on stdin.
        assertTrue(hostSource.contains("\"HOME\""))
        assertTrue(hostSource.contains("\"TMPDIR\""))
        assertFalse("no JWT secret may be passed through the environment", hostSource.contains("\"JWT_SECRET"))
    }

    @Test
    fun `the server runs only arm64-v8a devices`() {
        assertTrue(hostSource.contains("REQUIRED_ABI = \"arm64-v8a\""))
        assertTrue(hostSource.contains("Build.SUPPORTED_ABIS"))
    }
}
