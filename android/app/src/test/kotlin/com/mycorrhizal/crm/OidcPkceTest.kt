package com.mycorrhizal.crm

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test

// Issue #965: the PKCE primitives are pure JVM code, so they run on the plain
// unit-test runner (no Robolectric host needed).
class OidcPkceTest {

    // RFC 7636 Appendix B: the S256 transform must be exactly this. If the
    // hash, the base64url alphabet, or the (absent) padding ever drifts, every
    // real exchange starts failing closed — this pins the cause.
    private val rfcVerifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
    private val rfcChallenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

    @Test
    fun `challenge matches the RFC 7636 S256 vector`() {
        assertEquals(rfcChallenge, OidcPkce.challenge(rfcVerifier))
    }

    @Test
    fun `challenge is deterministic for the same verifier`() {
        val verifier = OidcPkce.generateVerifier()
        assertEquals(OidcPkce.challenge(verifier), OidcPkce.challenge(verifier))
    }

    @Test
    fun `generated verifier is 43 base64url characters`() {
        val verifier = OidcPkce.generateVerifier()
        assertEquals(43, verifier.length)
        assertTrue(
            "verifier must be base64url (RFC 7636 §4.1)",
            verifier.all { it.isLetterOrDigit() || it == '-' || it == '_' },
        )
        // ...and its challenge is the other 43-char half of the pair.
        assertEquals(43, OidcPkce.challenge(verifier).length)
    }

    @Test
    fun `each generated verifier and state is distinct`() {
        assertNotEquals(OidcPkce.generateVerifier(), OidcPkce.generateVerifier())
        assertNotEquals(OidcPkce.generateState(), OidcPkce.generateState())
    }

    @Test
    fun `verifyState accepts only an exact match`() {
        assertTrue(OidcPkce.verifyState("state-abc", "state-abc"))
        assertFalse(OidcPkce.verifyState("state-abc", "state-abd"))
        assertFalse(OidcPkce.verifyState("state-abc", "state-abc-longer"))
    }

    @Test
    fun `verifyState fails closed for a missing or stale pending state`() {
        assertFalse(OidcPkce.verifyState(null, "returned"))
        assertFalse(OidcPkce.verifyState("", "returned"))
        assertFalse(OidcPkce.verifyState("pending", null))
        assertFalse(OidcPkce.verifyState("pending", ""))
        assertFalse(OidcPkce.verifyState(null, null))
    }
}
