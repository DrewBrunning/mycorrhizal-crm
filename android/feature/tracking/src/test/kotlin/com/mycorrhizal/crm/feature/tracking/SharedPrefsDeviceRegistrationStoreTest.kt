package com.mycorrhizal.crm.feature.tracking

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
class SharedPrefsDeviceRegistrationStoreTest {

    private lateinit var context: Context
    private lateinit var store: SharedPrefsDeviceRegistrationStore

    @Before
    fun setup() {
        context = ApplicationProvider.getApplicationContext()
        // SharedPreferences (unlike DataStore) is a direct, synchronous store with no
        // process-lifetime in-memory cache layer, but still clear explicitly for isolation.
        context.getSharedPreferences("device_registration", Context.MODE_PRIVATE).edit().clear().apply()
        store = SharedPrefsDeviceRegistrationStore(context)
    }

    @Test
    fun `loadDeviceId is null before anything is saved`() {
        assertNull(store.loadDeviceId())
    }

    @Test
    fun `saveDeviceId then loadDeviceId round-trips`() {
        store.saveDeviceId(9)

        assertEquals(9, store.loadDeviceId())
    }

    @Test
    fun `clearDeviceId removes the stored id`() {
        store.saveDeviceId(9)

        store.clearDeviceId()

        assertNull(store.loadDeviceId())
    }

    @Test
    fun `a stored value of zero or less is treated as absent`() {
        context.getSharedPreferences("device_registration", Context.MODE_PRIVATE)
            .edit().putInt("fcm_device_registration_id", 0).apply()
        assertNull(store.loadDeviceId())

        context.getSharedPreferences("device_registration", Context.MODE_PRIVATE)
            .edit().putInt("fcm_device_registration_id", -1).apply()
        assertNull(store.loadDeviceId())
    }
}
