package com.mycorrhizal.crm.e2e

import androidx.test.ext.junit.runners.AndroidJUnit4
import com.mycorrhizal.crm.MainActivity
import com.mycorrhizal.crm.feature.tracking.DeviceRegistrationManager
import com.mycorrhizal.crm.feature.tracking.DeviceRegistrationStore
import com.mycorrhizal.crm.feature.tracking.FcmAvailability
import com.mycorrhizal.crm.feature.tracking.FcmTokenSource
import com.mycorrhizal.crm.network.ApiClient
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import java.util.UUID

/**
 * ANDROID-04 (issue #481): the FCM device-registration lifecycle, driven
 * against the real docker-compose.test.yml backend (issue #238's harness)
 * rather than a mocked [ApiClient] — the important failure mode this covers
 * (a rotation silently accumulating dead rows instead of replacing one) is a
 * "how many rows does the server actually have" question that a mocked-API
 * unit test cannot answer.
 *
 * [DeviceRegistrationManager] is constructed directly here (not via Hilt)
 * against the real, already-authenticated [apiClient] singleton
 * ([E2eBaseTest.e2eSetUp] logs in before every test), with:
 *  - [fcmAvailability] — a fake always returning true. The CI test build has
 *    no `google-services.json`, so the real `FirebaseFcmAvailability` would
 *    make every registration here a no-op and prove nothing.
 *  - an in-memory [DeviceRegistrationStore] per "device" — independent
 *    instances simulate independent physical installs sharing one account.
 *  - a controllable [FcmTokenSource] fake, so rotation is just a second
 *    `register(tokenOverride = ...)` call rather than needing a real FCM
 *    token refresh.
 *
 * What this suite deliberately does NOT re-cover, because it's already
 * pinned elsewhere:
 *  - Firebase/Play-Services-unavailable and token-fetch-failure degrading to
 *    a no-op: `DeviceRegistrationManagerTest` (Robolectric).
 *  - Login/logout *wiring* (a session transition calling register()/delete()):
 *    `DeviceRegistrationViewModelTest` (Robolectric).
 *  - Server-side stale-token detection marking a registration dead rather
 *    than retrying forever: `TestSendFCMMessage_StaleToken` and
 *    `TestSendReminders_FCMStaleRegistrationDropped` in
 *    `backend/services/notification_service_test.go` — exercising this live
 *    would need real FCM credentials the E2E backend doesn't have.
 */
@RunWith(AndroidJUnit4::class)
class NotificationDeviceLifecycleTest : E2eBaseTest() {

    private val fcmAvailability = FcmAvailability { true }
    private val createdDeviceIds = mutableListOf<Int>()

    /** The app's real, already-authenticated singleton. */
    private val apiClient: ApiClient
        get() = (compose.activity as MainActivity).apiClient

    @After
    fun deviceLifecycleTearDown() {
        createdDeviceIds.forEach { id -> runCatching { runBlocking { apiClient.deleteDevice(id) } } }
        createdDeviceIds.clear()
    }

    /** A fresh manager over an in-memory store — one simulated physical device. */
    private fun newManager(tokenSource: FcmTokenSource): DeviceRegistrationManager =
        DeviceRegistrationManager(
            apiClient = apiClient,
            availability = fcmAvailability,
            store = InMemoryDeviceRegistrationStore(),
            context = compose.activity.applicationContext,
            fcmToken = tokenSource,
        )

    private fun uniqueToken(label: String): String =
        "e2e-$label-${UUID.randomUUID().toString().replace("-", "").take(12)}"

    private suspend fun currentDevices() = apiClient.listDeviceRegistrations().getOrThrow()

    @Test
    fun freshRegistrationAppearsOnTheServer() = runBlocking {
        val token = uniqueToken("fresh")
        val manager = newManager(FcmTokenSource { token })

        val result = manager.register()

        assertTrue(result.isSuccess)
        val devices = currentDevices()
        val registered = devices.firstOrNull { it.token == token }
        assertTrue("expected a registered device for token $token, got $devices", registered != null)
        createdDeviceIds += registered!!.id
        assertTrue(registered.client == "fcm")
    }

    @Test
    fun rotationReplacesRatherThanAccumulates() = runBlocking {
        val tokenA = uniqueToken("rot-a")
        val tokenB = uniqueToken("rot-b")
        val manager = newManager(FcmTokenSource { tokenA })

        assertTrue(manager.register().isSuccess)
        val deviceIdA = currentDevices().first { it.token == tokenA }.id

        // Simulate onNewToken: the manager registers the override, not the
        // (now-fetched-only-once) token source's original value.
        assertTrue(manager.register(tokenOverride = tokenB).isSuccess)

        val finalDevices = currentDevices()
        val deviceB = finalDevices.firstOrNull { it.token == tokenB }
        assertTrue("expected the rotated token to be registered, got $finalDevices", deviceB != null)
        createdDeviceIds += deviceB!!.id

        // The important assertion: rotation must not leave the old row behind.
        assertFalse(
            "rotation must delete the old registration (id=$deviceIdA), not accumulate it",
            finalDevices.any { it.id == deviceIdA },
        )
    }

    @Test
    fun logoutRemovesTheRegistration() = runBlocking {
        val token = uniqueToken("logout")
        val manager = newManager(FcmTokenSource { token })

        assertTrue(manager.register().isSuccess)
        val deviceId = currentDevices().first { it.token == token }.id

        assertTrue(manager.delete().isSuccess)

        assertFalse(
            "expected device $deviceId to be gone after delete()",
            currentDevices().any { it.id == deviceId },
        )
    }

    @Test
    fun multiDeviceRegistrationsAreIndependent() = runBlocking {
        val tokenA = uniqueToken("multi-a")
        val tokenB = uniqueToken("multi-b")
        val deviceA = newManager(FcmTokenSource { tokenA })
        val deviceB = newManager(FcmTokenSource { tokenB })

        assertTrue(deviceA.register().isSuccess)
        assertTrue(deviceB.register().isSuccess)

        val devices = currentDevices()
        val idA = devices.first { it.token == tokenA }.id
        val idB = devices.first { it.token == tokenB }.id
        createdDeviceIds += idA
        createdDeviceIds += idB

        // Removing one device must not affect the other.
        assertTrue(deviceA.delete().isSuccess)
        val afterRemovingA = currentDevices().map { it.id }
        assertFalse(afterRemovingA.contains(idA))
        assertTrue("removing device A must not remove device B", afterRemovingA.contains(idB))
    }
}

/** A plain in-memory [DeviceRegistrationStore] — one instance per simulated
 *  physical device, so multiple managers in the same test never share state
 *  (unlike the real SharedPreferences-backed store, which is one per app). */
private class InMemoryDeviceRegistrationStore : DeviceRegistrationStore {
    private var deviceId: Int? = null

    override fun loadDeviceId(): Int? = deviceId
    override fun saveDeviceId(id: Int) {
        deviceId = id
    }
    override fun clearDeviceId() {
        deviceId = null
    }
}
