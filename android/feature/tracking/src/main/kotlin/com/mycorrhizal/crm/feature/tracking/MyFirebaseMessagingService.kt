package com.mycorrhizal.crm.feature.tracking

import com.google.firebase.messaging.FirebaseMessagingService
import com.google.firebase.messaging.RemoteMessage
import dagger.hilt.android.AndroidEntryPoint
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch
import javax.inject.Inject

/**
 * M5 §5a (issue #152): the FCM entry point. The server pushes reminder
 * notifications carrying a `data` payload; `onMessageReceived` parses it and
 * posts on the local channel with the deep-link content intent and the
 * (reminder_id, due_at) idempotent notification id — the same id the polling
 * worker targets, so the poll boundary collapses to one notification.
 *
 * The WorkManager polling workers are untouched: this service short-circuits
 * the common case and degrades to a no-op when Firebase is unavailable, keeping
 * polling as the safety net and the only path on de-Googled devices.
 */
@AndroidEntryPoint
class MyFirebaseMessagingService : FirebaseMessagingService() {

    @Inject
    lateinit var dispatcher: PushNotificationDispatcher

    @Inject
    lateinit var deviceRegistration: DeviceRegistrationManager

    private val ioScope = CoroutineScope(SupervisorJob() + Dispatchers.IO)

    override fun onMessageReceived(message: RemoteMessage) {
        val push = PushMessageParser.parse(
            data = message.data,
            title = message.notification?.title,
            body = message.notification?.body,
        ) ?: return
        dispatcher.dispatch(applicationContext, push)
    }

    /**
     * A refreshed token means this install's registration is stale server-side;
     * re-register (idempotent — the server reassigns the (client, token) row).
     * Guarded by FcmAvailability inside the manager, so a token refresh on an
     * app that lost its Firebase config is a no-op rather than an error.
     */
    override fun onNewToken(token: String) {
        ioScope.launch {
            deviceRegistration.register(tokenOverride = token)
        }
    }

    override fun onDestroy() {
        ioScope.cancel()
        super.onDestroy()
    }
}
