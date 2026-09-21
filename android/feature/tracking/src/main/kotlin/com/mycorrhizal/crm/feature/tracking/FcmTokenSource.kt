package com.mycorrhizal.crm.feature.tracking

/**
 * M5 §5a (issue #152): supplies the FCM registration token. Separated from
 * [DeviceRegistrationManager] so the availability guard can be pinned: when
 * Firebase is unavailable the manager must short-circuit BEFORE this is ever
 * invoked, and a token-fetch failure is itself a degrade-to-polling path.
 *
 * Issue #1133: the concrete Firebase-backed source lives in `:app`'s `src/fcm`
 * (obtainium/play flavors); the `foss` flavor binds a no-op that is never
 * reached. This interface keeps `:feature:tracking` free of proprietary deps.
 */
fun interface FcmTokenSource {
    suspend fun token(): String
}
