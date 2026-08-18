package com.mycorrhizal.crm.feature.tracking

import com.google.firebase.messaging.FirebaseMessaging
import kotlinx.coroutines.tasks.await
import javax.inject.Inject

/**
 * M5 §5a (issue #152): supplies the FCM registration token. Separated from
 * [DeviceRegistrationManager] so the availability guard can be pinned: when
 * Firebase is unavailable the manager must short-circuit BEFORE this is ever
 * invoked, and a token-fetch failure is itself a degrade-to-polling path.
 */
fun interface FcmTokenSource {
    suspend fun token(): String
}

/** Fetches the token from Firebase Messaging itself. */
class FirebaseFcmTokenSource @Inject constructor() : FcmTokenSource {
    override suspend fun token(): String = FirebaseMessaging.getInstance().token.await()
}
