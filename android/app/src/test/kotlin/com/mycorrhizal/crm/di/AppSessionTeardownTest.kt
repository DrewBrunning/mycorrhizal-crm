package com.mycorrhizal.crm.di

import com.mycorrhizal.crm.data.auth.CurrentSessionRevoker
import com.mycorrhizal.crm.feature.tracking.DeviceRegistrationManager
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.Test

/**
 * Issue #957: [AppSessionTeardown] composes the two independent teardown
 * steps — one failing must never skip or fail the other, or a network
 * hiccup on the FCM dereg would silently drop the session revoke too (and
 * vice versa), leaving the JWT usable past its intended end.
 */
class AppSessionTeardownTest {

    @Test
    fun `runs both steps`() = runTest {
        val deviceRegistration = mockk<DeviceRegistrationManager>()
        coEvery { deviceRegistration.delete() } returns Result.success(Unit)
        val sessionRevoker = mockk<CurrentSessionRevoker>()
        coEvery { sessionRevoker.revoke() } returns Unit

        AppSessionTeardown(deviceRegistration, sessionRevoker).beforeClear()

        coVerify(exactly = 1) { deviceRegistration.delete() }
        coVerify(exactly = 1) { sessionRevoker.revoke() }
    }

    @Test
    fun `the session revoke still runs when FCM deregistration throws`() = runTest {
        val deviceRegistration = mockk<DeviceRegistrationManager>()
        coEvery { deviceRegistration.delete() } throws RuntimeException("network unreachable")
        val sessionRevoker = mockk<CurrentSessionRevoker>()
        coEvery { sessionRevoker.revoke() } returns Unit

        AppSessionTeardown(deviceRegistration, sessionRevoker).beforeClear()

        coVerify(exactly = 1) { sessionRevoker.revoke() }
    }

    @Test
    fun `FCM deregistration still ran when the session revoke throws`() = runTest {
        val deviceRegistration = mockk<DeviceRegistrationManager>()
        coEvery { deviceRegistration.delete() } returns Result.success(Unit)
        val sessionRevoker = mockk<CurrentSessionRevoker>()
        coEvery { sessionRevoker.revoke() } throws RuntimeException("network unreachable")

        // Must not throw out of beforeClear -- DefaultSessionManager.clearSession
        // must not be blocked by this.
        AppSessionTeardown(deviceRegistration, sessionRevoker).beforeClear()

        coVerify(exactly = 1) { deviceRegistration.delete() }
    }
}
