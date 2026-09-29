package com.mycorrhizal.crm.passkey

import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * Issue #1293 / ADR 0034 Decision 4: the F-Droid `foss` flavor must not pick up
 * Google Play services for passkeys. Only the GMS-free core
 * `androidx.credentials` artifact is on its classpath; the Play services
 * provider (`credentials-play-services-auth`, which drags in play-services-auth
 * / -fido) is scoped to obtainium/play in app/build.gradle.kts.
 * The APK-level twin is the "FOSS APK is Firebase-free" step in android-tests.yml.
 */
class FossFlavorGmsFreeTest {

    private fun loadable(name: String): Boolean =
        runCatching { Class.forName(name) }.isSuccess

    @Test
    fun `the core Credential Manager API is present`() {
        assertNotNull(runCatching { Class.forName("androidx.credentials.CredentialManager") }.getOrNull())
    }

    @Test
    fun `the Play services credential provider is absent`() {
        assertFalse(loadable("androidx.credentials.playservices.CredentialProviderPlayServicesImpl"))
    }

    @Test
    fun `no Google Play services or Firebase classes are on the foss classpath`() {
        for (name in listOf(
            "com.google.android.gms.common.GoogleApiAvailability",
            "com.google.android.gms.fido.Fido",
            "com.google.android.gms.auth.api.identity.Identity",
            "com.google.firebase.messaging.FirebaseMessaging",
        )) {
            assertNull("$name must not be on the foss classpath", runCatching { Class.forName(name) }.getOrNull())
        }
    }
}
