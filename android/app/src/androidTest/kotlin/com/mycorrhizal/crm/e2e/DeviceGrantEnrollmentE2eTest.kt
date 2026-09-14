package com.mycorrhizal.crm.e2e

import androidx.test.ext.junit.runners.AndroidJUnit4
import com.mycorrhizal.crm.MainActivity
import com.mycorrhizal.crm.network.ApiClient
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import java.util.UUID

/**
 * Issue #722: the fully-biometric-login device-grant round trip, driven through
 * the app's real [ApiClient] against the docker-compose.test.yml backend.
 *
 * The biometric prompt itself cannot be automated, so the UI half is covered by
 * `BiometricEnrollmentViewModelTest`; what this test owns is the wire contract
 * that the UI cannot reach: `createDeviceGrant` — the call
 * `DeviceGrantManager.enroll` makes — must actually mint a grant, and that grant
 * must exchange for a fresh session. It is deliberately not a mocked-API unit
 * test: the bug this guards against (#722's middleware double-read returned 400
 * INVALID_INPUT / EOF on a valid body) only exists against the real route stack,
 * which a fake [ApiClient] cannot reproduce.
 *
 * The suite's shared seed account is already logged in by
 * [E2eBaseTest.e2eSetUp], so [apiClient] carries a valid bearer token.
 */
@RunWith(AndroidJUnit4::class)
class DeviceGrantEnrollmentE2eTest : E2eBaseTest() {

    private val createdGrantIds = mutableListOf<Long>()

    /** The app's real, already-authenticated singleton. */
    private val apiClient: ApiClient
        get() = (compose.activity as MainActivity).apiClient

    @After
    fun deviceGrantTearDown() {
        createdGrantIds.forEach { id -> runCatching { runBlocking { apiClient.revokeDeviceGrant(id) } } }
        createdGrantIds.clear()
    }

    @Test
    fun enrollThenExchangeMintsASessionThroughTheRealClient() = runBlocking {
        val label = "e2e-${UUID.randomUUID().toString().replace("-", "").take(8)}"

        val created = apiClient.createDeviceGrant(label).getOrThrow()
        val token = created.token
        assertTrue("enroll must return a one-time grant token", !token.isNullOrBlank())
        assertTrue("enroll must return the grant's id", created.id > 0)
        createdGrantIds += created.id

        // Possession of the stored grant exchanges for a fresh session JWT —
        // the second half of DeviceGrantManager's flow.
        val exchanged = apiClient.exchangeDeviceSession(token!!)
        assertTrue("device-grant exchange must mint a session, got $exchanged", exchanged.isSuccess)
    }
}
