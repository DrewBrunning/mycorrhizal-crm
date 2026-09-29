package com.mycorrhizal.crm.passkey

import org.junit.Assert.assertNotNull
import org.junit.Test

/**
 * Issue #1293 / ADR 0034: the obtainium and play flavors DO bundle the Play
 * services credential provider (runtime-only), which is what back-fills
 * Credential Manager below Android 14. Guards against the flavor-scoped
 * dependency being dropped — and is the mirror of the foss flavor's
 * FossFlavorGmsFreeTest, so the two can't both pass by the dependency having
 * been removed everywhere.
 */
class FcmFlavorPasskeyProviderTest {
    @Test
    fun `the Play services credential provider is bundled`() {
        assertNotNull(
            runCatching { Class.forName("androidx.credentials.playservices.CredentialProviderPlayServicesImpl") }.getOrNull(),
        )
    }
}
