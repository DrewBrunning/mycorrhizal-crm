package com.mycorrhizal.crm.data.session

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

// Issue #965: the pending-request stale-window is pure logic, tested without a
// Keystore host (which the encrypted store itself needs).
class OidcPendingRequestTest {

    private val created = 1_000L

    @Test
    fun `a request within the TTL is not expired`() {
        val request = OidcPendingRequest("state", "verifier", created)
        assertFalse(request.isExpired(created + OIDC_PENDING_REQUEST_TTL_MILLIS))
        assertFalse(request.isExpired(created))
    }

    @Test
    fun `a request past the TTL is expired`() {
        val request = OidcPendingRequest("state", "verifier", created)
        assertTrue(request.isExpired(created + OIDC_PENDING_REQUEST_TTL_MILLIS + 1))
    }
}
