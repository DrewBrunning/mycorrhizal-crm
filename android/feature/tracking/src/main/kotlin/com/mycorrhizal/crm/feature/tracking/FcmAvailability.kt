package com.mycorrhizal.crm.feature.tracking

import android.content.Context

/**
 * M5 §5a (issue #152): reports whether push (FCM) is usable on this
 * device/install. The app is self-hosted-friendly by design: a build without a
 * `google-services.json` (no Firebase project configured) has no initialized
 * `FirebaseApp`, and a de-Googled device may have no usable Firebase runtime —
 * in both cases FCM is simply not available. Everything that touches Firebase
 * gates on this so the WorkManager polling workers stay the sole push path
 * instead of the app crashing or spinning forever on an unavailable token.
 *
 * A `fun interface` rather than a concrete class for two reasons:
 *  - the CI instrumented suite (issue #238) has no real `google-services.json`,
 *    so a real availability check would make every registration a no-op there
 *    and prove nothing — ANDROID-04 (#481) fakes this with a plain lambda to
 *    exercise [DeviceRegistrationManager] against the real backend instead;
 *  - issue #1133: the concrete implementation is distribution-flavor-specific.
 *    The `obtainium`/`play` flavors bind a Firebase-backed implementation
 *    (in `:app`'s `src/fcm`), while the `foss` (F-Droid) flavor binds a
 *    permanently-unavailable no-op so its APK carries no proprietary SDK.
 *    This module stays Firebase-free for either.
 */
fun interface FcmAvailability {
    fun isAvailable(context: Context): Boolean
}
