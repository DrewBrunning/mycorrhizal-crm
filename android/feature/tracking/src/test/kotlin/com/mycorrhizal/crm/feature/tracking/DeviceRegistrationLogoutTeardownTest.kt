package com.mycorrhizal.crm.feature.tracking

import android.app.Application
import android.content.Context
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.data.session.DefaultSessionManager
import com.mycorrhizal.crm.data.session.SessionPrefsStorage
import com.mycorrhizal.crm.data.session.SessionTeardown
import com.mycorrhizal.crm.data.session.TokenStorage
import com.mycorrhizal.crm.domain.repository.SessionState
import com.mycorrhizal.crm.network.ApiClient
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/**
 * Issue #957 (finding #1): drives a REAL logout through a REAL
 * [DefaultSessionManager] and a REAL [DeviceRegistrationManager] — only
 * [ApiClient], the actual network boundary, is mocked. This is the test the
 * issue asked for explicitly: [DeviceRegistrationViewModelTest] mocks
 * [DeviceRegistrationManager] itself away, so it could never see the
 * original bug, which was entirely about ordering — `clearSession()` used
 * to null the bearer token and THEN (via the reactive ViewModel collector)
 * trigger `delete()`, so the deregistration request went out with no
 * `Authorization` header and 401ed, leaving the push registration live on a
 * logged-out device.
 *
 * The fix moved deregistration into [SessionTeardown], invoked by
 * `clearSession()` itself while the bearer is still valid; this pins that a
 * real [DeviceRegistrationManager.delete] call made from that hook actually
 * observes the still-live token.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
class DeviceRegistrationLogoutTeardownTest {

    private val context = ApplicationProvider.getApplicationContext<Context>()

    @Test
    fun `logout deregisters the FCM device while the session is still authenticated`() = runTest {
        val apiClient = mockk<ApiClient>()
        val availability = mockk<FcmAvailability>()
        every { availability.isAvailable(context) } returns true
        val store = mockk<DeviceRegistrationStore>(relaxed = true)
        every { store.loadDeviceId() } returns 42

        var tokenSeenByDeleteDevice: String? = null
        lateinit var sessionManager: DefaultSessionManager
        coEvery { apiClient.deleteDevice(42) } answers {
            // The same thing AuthInterceptor reads to attach the
            // Authorization header on the real request this call stands in
            // for.
            tokenSeenByDeleteDevice = sessionManager.bearerToken()
            Result.success(Unit)
        }

        val deviceRegistration = DeviceRegistrationManager(
            apiClient,
            availability,
            store,
            context,
            FcmTokenSource { "fetched-token" },
        )
        sessionManager = DefaultSessionManager(
            InMemoryTokenStorage(),
            InMemorySessionPrefsStorage(),
            sessionTeardown = SessionTeardown { deviceRegistration.delete() },
        )
        sessionManager.setSession(
            serverUrl = "https://crm.example.com",
            token = "jwt-1",
            state = SessionState(userId = 7),
        )

        sessionManager.clearSession()

        assertEquals(
            "deleteDevice must run while the session's own bearer is still live, not after clearSession " +
                "has already nulled it (issue #957 finding #1)",
            "jwt-1",
            tokenSeenByDeleteDevice,
        )
        coVerify { store.clearDeviceId() }
        assertNull("the session must still end up cleared", sessionManager.bearerToken())
    }

    /** Minimal in-memory [TokenStorage] — core:data's own FakeTokenStorage
     *  lives in its test source set, not visible from this module's tests. */
    private class InMemoryTokenStorage : TokenStorage {
        private var stored: String? = null
        override suspend fun save(token: String) { stored = token }
        override suspend fun load(): String? = stored
        override suspend fun clear() { stored = null }
    }

    /** Minimal in-memory [SessionPrefsStorage] — see [InMemoryTokenStorage]. */
    private class InMemorySessionPrefsStorage : SessionPrefsStorage {
        private var stored: String? = null
        override suspend fun save(serverUrl: String?) { stored = serverUrl }
        override suspend fun loadServerUrl(): String? = stored
        override suspend fun clear() { stored = null }
    }
}
