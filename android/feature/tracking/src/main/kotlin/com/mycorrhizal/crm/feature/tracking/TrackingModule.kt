package com.mycorrhizal.crm.feature.tracking

import dagger.Binds
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent

/**
 * M5 §5a (issue #152): DI bindings for the tracking/push module.
 */
@Module
@InstallIn(SingletonComponent::class)
abstract class TrackingModule {

    @Binds
    abstract fun bindDeviceRegistrationStore(
        impl: SharedPrefsDeviceRegistrationStore,
    ): DeviceRegistrationStore
}

@Module
@InstallIn(SingletonComponent::class)
object TrackingProvidesModule {

    @Provides
    fun provideFcmTokenSource(impl: FirebaseFcmTokenSource): FcmTokenSource = impl

    @Provides
    fun provideFcmAvailability(impl: FirebaseFcmAvailability): FcmAvailability = impl
}
