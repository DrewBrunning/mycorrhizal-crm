package com.mycorrhizal.crm.feature.tracking

import dagger.Binds
import dagger.Module
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent

/**
 * M5 §5a (issue #152): DI bindings for the tracking/push module.
 *
 * Issue #1133: the [FcmTokenSource]/[FcmAvailability] bindings are NOT here.
 * Their implementations are distribution-flavor-specific (Firebase for
 * obtainium/play, a no-op for FOSS), so they live in `:app`'s per-flavor
 * source sets — keeping this module and its APK contribution free of the
 * proprietary Firebase SDK.
 */
@Module
@InstallIn(SingletonComponent::class)
abstract class TrackingModule {

    @Binds
    abstract fun bindDeviceRegistrationStore(
        impl: SharedPrefsDeviceRegistrationStore,
    ): DeviceRegistrationStore

    // Issue #721: the OS-grant query seam and the grant-time catch-up enqueue
    // seam, both consumed by the Settings feature's ViewModel.
    @Binds
    abstract fun bindPermissionChecker(impl: AndroidPermissionChecker): PermissionChecker

    @Binds
    abstract fun bindTrackingCatchUpScheduler(
        impl: TrackingCatchUpSchedulerImpl,
    ): TrackingCatchUpScheduler
}
