package com.mycorrhizal.crm.feature.tracking

import android.content.Context
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject

/**
 * Remembers the server-side id of this install's device registration, so logout
 * can `DELETE /notifications/devices/:id`. Plain SharedPreferences (not a
 * credential); lost rows are harmless because re-registering the same token
 * reassigns the server row to the new owner.
 */
interface DeviceRegistrationStore {
    fun loadDeviceId(): Int?
    fun saveDeviceId(id: Int)
    fun clearDeviceId()
}

/** SharedPreferences-backed [DeviceRegistrationStore]. */
class SharedPrefsDeviceRegistrationStore @Inject constructor(
    @ApplicationContext private val context: Context,
) : DeviceRegistrationStore {

    private val prefs = context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)

    override fun loadDeviceId(): Int? =
        prefs.getInt(KEY_DEVICE_ID, -1).takeIf { it > 0 }

    override fun saveDeviceId(id: Int) {
        prefs.edit().putInt(KEY_DEVICE_ID, id).apply()
    }

    override fun clearDeviceId() {
        prefs.edit().remove(KEY_DEVICE_ID).apply()
    }

    private companion object {
        const val PREFS_NAME = "device_registration"
        const val KEY_DEVICE_ID = "fcm_device_registration_id"
    }
}
