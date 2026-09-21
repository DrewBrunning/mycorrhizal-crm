package com.mycorrhizal.crm.push

import com.google.firebase.messaging.FirebaseMessaging
import com.mycorrhizal.crm.feature.tracking.FcmTokenSource
import javax.inject.Inject
import kotlinx.coroutines.tasks.await

/**
 * Firebase-backed [FcmTokenSource] for the `obtainium` and `play` distribution
 * flavors (issue #1133). The `foss` flavor binds a no-op that is never reached
 * — `DeviceRegistrationManager` gates on FcmAvailability first, and the no-op
 * reports unavailable.
 */
class FirebaseFcmTokenSource @Inject constructor() : FcmTokenSource {
    override suspend fun token(): String = FirebaseMessaging.getInstance().token.await()
}
