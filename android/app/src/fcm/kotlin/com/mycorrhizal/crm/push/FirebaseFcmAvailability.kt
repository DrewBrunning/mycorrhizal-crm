package com.mycorrhizal.crm.push

import android.content.Context
import com.google.firebase.FirebaseApp
import com.mycorrhizal.crm.feature.tracking.FcmAvailability
import javax.inject.Inject

/**
 * Firebase-backed [FcmAvailability] for the `obtainium` and `play` distribution
 * flavors (issue #1133). The `foss` flavor binds a no-op instead (see
 * [FossPushModule]), so the Firebase SDK never reaches that variant's APK.
 *
 * True when at least one FirebaseApp is initialized. An absent
 * google-services.json leaves the app with zero apps (the Firebase init
 * provider logs a warning and skips), which is exactly the "no Firebase
 * project" signal this is for. Play Services absence on de-Googled devices
 * surfaces the same way (FirebaseApp cannot initialize without it).
 */
class FirebaseFcmAvailability @Inject constructor() : FcmAvailability {
    override fun isAvailable(context: Context): Boolean = FirebaseApp.getApps(context).isNotEmpty()
}
