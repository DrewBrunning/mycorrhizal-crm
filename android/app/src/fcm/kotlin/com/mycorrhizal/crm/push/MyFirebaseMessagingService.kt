package com.mycorrhizal.crm.push

import android.content.Context
import com.google.firebase.messaging.FirebaseMessagingService
import com.google.firebase.messaging.RemoteMessage
import com.mycorrhizal.crm.feature.tracking.PushMessageParser
import com.mycorrhizal.crm.feature.tracking.PushNotificationDispatcher
import com.mycorrhizal.crm.feature.tracking.TrackingWorkerScheduler
import dagger.hilt.android.AndroidEntryPoint
import javax.inject.Inject

/**
 * M5 §5a (issue #152): the FCM entry point. The server pushes reminder
 * notifications carrying a `data` payload; `onMessageReceived` parses it and
 * posts on the local channel with the deep-link content intent and the
 * (reminder_id, due_at) idempotent notification id — the same id the polling
 * worker targets, so the poll boundary collapses to one notification.
 *
 * Issue #1133: this class lives in `:app`'s `src/fcm` source set, shared by the
 * `obtainium` and `play` distribution flavors only — the F-Droid (`foss`)
 * flavor neither compiles it nor carries the Firebase SDK. It is registered in
 * `src/fcm/AndroidManifest.xml` for exactly those two flavors.
 *
 * The WorkManager polling workers are untouched: this service short-circuits
 * the common case and degrades to a no-op when Firebase is unavailable, keeping
 * polling as the safety net and the only path on de-Googled devices.
 */
@AndroidEntryPoint
class MyFirebaseMessagingService : FirebaseMessagingService() {

    @Inject
    lateinit var dispatcher: PushNotificationDispatcher

    /** Seam for tests; production hands the token to a persisted WorkManager job. */
    internal var enqueueTokenRegistration: (Context, String) -> Unit =
        TrackingWorkerScheduler::enqueueFcmTokenRegistration

    override fun onMessageReceived(message: RemoteMessage) {
        val push = PushMessageParser.parse(
            data = message.data,
            title = message.notification?.title,
            body = message.notification?.body
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
        // Issue #2066: not an unstructured scope tied to this service's lifetime.
        enqueueTokenRegistration(applicationContext, token)
    }
}
