package com.mycorrhizal.crm.push

import com.mycorrhizal.crm.feature.tracking.FcmAvailability
import com.mycorrhizal.crm.feature.tracking.FcmTokenSource
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent

/**
 * Issue #1133: Hilt bindings for the `foss` (F-Droid) distribution flavor.
 * That variant must carry no proprietary dependencies, so it binds a push
 * seam that is permanently unavailable rather than the Firebase-backed
 * implementations compiled into `src/fcm` for `obtainium`/`play`.
 *
 * This is not a degraded hack — it is the documented FOSS posture: Firebase
 * Cloud Messaging is only a latency fast-path, and the WorkManager polling
 * workers (`ReminderNotificationWorker`, `CadenceCheckWorker`,
 * `BirthdayCheckWorker`) are the sole and always-present push path. Reporting
 * unavailable here makes `DeviceRegistrationManager` a successful no-op, so
 * the FOSS build never enrolls a device and never touches the network for
 * push registration.
 */
@Module
@InstallIn(SingletonComponent::class)
object FossPushModule {

    @Provides
    fun provideFcmAvailability(): FcmAvailability = FcmAvailability { false }

    /**
     * Never invoked: `DeviceRegistrationManager.register` checks
     * [FcmAvailability] first and returns before touching this. Throwing here
     * turns a future ordering regression into a loud failure rather than a
     * silent hang on a token that can never arrive.
     */
    @Provides
    fun provideFcmTokenSource(): FcmTokenSource = FcmTokenSource {
        error("FCM is not available in the FOSS build; notifications come from the WorkManager polling workers.")
    }
}
