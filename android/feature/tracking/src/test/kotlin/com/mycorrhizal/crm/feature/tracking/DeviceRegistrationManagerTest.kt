package com.mycorrhizal.crm.feature.tracking

import android.app.Application
import android.content.Context
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.model.network.DeviceRegistration
import com.mycorrhizal.crm.model.network.DeviceRegistrationInput
import com.mycorrhizal.crm.network.ApiClient
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import io.mockk.slot
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/**
 * M5 §5a (issue #152): the registration manager's degradation contract. The
 * fallback-to-polling guarantee is the important one — when Firebase is
 * unavailable (no google-services.json, or Play Services absent), registration
 * must short-circuit BEFORE the token source is ever touched and never reach
 * the API, so the WorkManager polling workers stay the sole push path.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
class DeviceRegistrationManagerTest {

    private val context = ApplicationProvider.getApplicationContext<Context>()
    private val fakeToken = FcmTokenSource { "fetched-token" }

    private fun manager(
        apiClient: ApiClient,
        availability: FcmAvailability,
        store: DeviceRegistrationStore,
        tokenSource: FcmTokenSource = fakeToken,
    ) = DeviceRegistrationManager(apiClient, availability, store, context, tokenSource)

    @Test
    fun `registration is a no-op when Firebase is unavailable`() = runTest {
        val apiClient = mockk<ApiClient>(relaxed = true)
        val availability = mockk<FcmAvailability>()
        every { availability.isAvailable(context) } returns false
        val tokenSource = mockk<FcmTokenSource>(relaxed = true)

        val result = manager(apiClient, availability, mockk<DeviceRegistrationStore>(relaxed = true), tokenSource)
            .register()

        assertTrue(result.isSuccess)
        // The guard must short-circuit before any Firebase work happens.
        coVerify(exactly = 0) { tokenSource.token() }
        coVerify(exactly = 0) { apiClient.registerDevice(any()) }
    }

    @Test
    fun `registration fetches the token and stores the returned device id`() = runTest {
        val apiClient = mockk<ApiClient>()
        coEvery { apiClient.registerDevice(any()) } returns
            Result.success(DeviceRegistration(id = 9, token = "fetched-token", client = "fcm"))
        val availability = mockk<FcmAvailability>()
        every { availability.isAvailable(context) } returns true
        val store = mockk<DeviceRegistrationStore>(relaxed = true)

        val result = manager(apiClient, availability, store).register()

        assertTrue(result.isSuccess)
        val captured = slot<DeviceRegistrationInput>()
        coVerify { apiClient.registerDevice(capture(captured)) }
        assertEquals("fetched-token", captured.captured.token)
        assertEquals("fcm", captured.captured.client)
        coVerify { store.saveDeviceId(9) }
    }

    @Test
    fun `a token fetch failure degrades to a no-op`() = runTest {
        val apiClient = mockk<ApiClient>(relaxed = true)
        val availability = mockk<FcmAvailability>()
        every { availability.isAvailable(context) } returns true
        val tokenSource = FcmTokenSource { error("no Play Services") }

        val result = manager(apiClient, availability, mockk<DeviceRegistrationStore>(relaxed = true), tokenSource)
            .register()

        assertTrue(result.isSuccess)
        coVerify(exactly = 0) { apiClient.registerDevice(any()) }
    }

    @Test
    fun `onNewToken registration posts the override token`() = runTest {
        val apiClient = mockk<ApiClient>()
        coEvery { apiClient.registerDevice(any()) } returns
            Result.success(DeviceRegistration(id = 5, token = "rotated-token", client = "fcm"))
        val availability = mockk<FcmAvailability>()
        every { availability.isAvailable(context) } returns true
        val tokenSource = mockk<FcmTokenSource>(relaxed = true)

        val result = manager(apiClient, availability, mockk<DeviceRegistrationStore>(relaxed = true), tokenSource)
            .register(tokenOverride = "rotated-token")

        assertTrue(result.isSuccess)
        val captured = slot<DeviceRegistrationInput>()
        coVerify { apiClient.registerDevice(capture(captured)) }
        assertEquals("rotated-token", captured.captured.token)
        // onNewToken passes its own token — the source is never consulted.
        coVerify(exactly = 0) { tokenSource.token() }
    }

    @Test
    fun `delete removes the stored registration`() = runTest {
        val apiClient = mockk<ApiClient>()
        coEvery { apiClient.deleteDevice(9) } returns Result.success(Unit)
        val availability = mockk<FcmAvailability>()
        val store = mockk<DeviceRegistrationStore>(relaxed = true)
        every { store.loadDeviceId() } returns 9

        val result = manager(apiClient, availability, store).delete()

        assertTrue(result.isSuccess)
        coVerify { apiClient.deleteDevice(9) }
        coVerify { store.clearDeviceId() }
    }

    @Test
    fun `delete with no stored device is a no-op`() = runTest {
        val apiClient = mockk<ApiClient>(relaxed = true)
        val availability = mockk<FcmAvailability>()
        val store = mockk<DeviceRegistrationStore>()
        every { store.loadDeviceId() } returns null

        val result = manager(apiClient, availability, store).delete()

        assertTrue(result.isSuccess)
        coVerify(exactly = 0) { apiClient.deleteDevice(any()) }
    }
}
