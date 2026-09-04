package com.mycorrhizal.crm.feature.tracking

import android.content.Context
import android.os.Build
import com.mycorrhizal.crm.model.network.DeviceRegistrationInput
import com.mycorrhizal.crm.network.ApiClient
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject

/**
 * M5 §5a (issue #152): registers this install's FCM token with the server on
 * login (and on token refresh) and deletes the registration on logout.
 *
 * Graceful degradation is the whole point: when Firebase is unavailable (no
 * google-services.json, or Play Services absent on a de-Googled device) every
 * operation is a successful no-op, so the WorkManager polling workers remain
 * the sole push path and nothing logs an error or throws. The availability
 * guard runs BEFORE the token source is touched, and a token-fetch failure is
 * itself a degrade-to-polling no-op.
 */
class DeviceRegistrationManager @Inject constructor(
    private val apiClient: ApiClient,
    private val availability: FcmAvailability,
    private val store: DeviceRegistrationStore,
    @ApplicationContext private val context: Context,
    private val fcmToken: FcmTokenSource,
) {

    /**
     * Registers the current FCM token (or [tokenOverride], from `onNewToken`)
     * as this user's device. No-op success when Firebase is unavailable.
     */
    suspend fun register(tokenOverride: String? = null): Result<Unit> {
        if (!availability.isAvailable(context)) return Result.success(Unit)
        val token = tokenOverride
            ?: runCatching { fcmToken.token() }.getOrElse {
                // A device with Firebase present but the token fetch failing is
                // still better served by the polling workers than by crashing.
                return Result.success(Unit)
            }
        val previousId = store.loadDeviceId()
        val device = apiClient.registerDevice(
            DeviceRegistrationInput(token = token, client = CLIENT_FCM, deviceLabel = deviceLabel()),
        ).getOrElse { return Result.failure(it) }
        store.saveDeviceId(device.id)
        // A rotated token registers as a brand-new server-side row (the unique
        // key is (client, token), not this physical install — see migration
        // 000019's comment) — delete the old row so rotation replaces the
        // registration instead of accumulating one per rotation (ANDROID-04,
        // issue #481). Best-effort: a failed cleanup here doesn't fail the
        // overall registration, since the new token is already live, and a
        // row this misses is still pruned once FCM reports it dead
        // (pushNotificationSender's stale-token path server-side).
        if (previousId != null && previousId != device.id) {
            apiClient.deleteDevice(previousId)
        }
        return Result.success(Unit)
    }

    /** Deletes this install's registration. No-op success when none is stored. */
    suspend fun delete(): Result<Unit> {
        val id = store.loadDeviceId() ?: return Result.success(Unit)
        return apiClient.deleteDevice(id).map { store.clearDeviceId() }
    }

    private fun deviceLabel(): String = listOfNotNull(Build.MANUFACTURER, Build.MODEL)
        .distinct()
        .joinToString(" ")
        .ifBlank { "Android" }

    companion object {
        const val CLIENT_FCM = "fcm"
    }
}
