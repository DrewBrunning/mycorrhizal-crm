package com.mycorrhizal.crm.feature.tracking

import android.content.Context
import com.google.firebase.FirebaseApp
import javax.inject.Inject

/**
 * M5 §5a (issue #152): reports whether Firebase Messaging is usable on this
 * device/install. The app is self-hosted-friendly by design: a build without a
 * `google-services.json` (no Firebase project configured) has no initialized
 * [FirebaseApp], and a de-Googled device may have no usable Firebase runtime —
 * in both cases `FirebaseApp.getApps()` is empty and FCM is simply not
 * available. Everything that touches Firebase gates on this so the WorkManager
 * polling workers stay the sole push path instead of the app crashing or
 * spinning forever on an unavailable token.
 *
 * A `fun interface` (mirroring [FcmTokenSource]) rather than a concrete class:
 * the CI instrumented suite (issue #238) has no real `google-services.json`,
 * so the real [FirebaseFcmAvailability] would make every registration a no-op
 * there and prove nothing — ANDROID-04 (#481) fakes this with a plain lambda
 * to exercise [DeviceRegistrationManager] against the real backend instead.
 */
fun interface FcmAvailability {
    fun isAvailable(context: Context): Boolean
}

/**
 * True when at least one FirebaseApp is initialized. An absent
 * google-services.json leaves the app with zero apps (the Firebase init
 * provider logs a warning and skips), which is exactly the "no Firebase
 * project" signal this is for. Play Services absence on de-Googled devices
 * surfaces the same way (FirebaseApp cannot initialize without it).
 */
class FirebaseFcmAvailability @Inject constructor() : FcmAvailability {
    override fun isAvailable(context: Context): Boolean = FirebaseApp.getApps(context).isNotEmpty()
}
