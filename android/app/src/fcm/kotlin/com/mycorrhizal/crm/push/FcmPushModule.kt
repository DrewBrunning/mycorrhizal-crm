package com.mycorrhizal.crm.push

import com.mycorrhizal.crm.feature.tracking.FcmAvailability
import com.mycorrhizal.crm.feature.tracking.FcmTokenSource
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent

/**
 * Issue #1133: Hilt bindings for the Firebase-backed push seams, compiled only
 * into the `obtainium` and `play` distribution flavors (this file is in
 * `src/fcm`, which those two flavors include). The `foss` flavor binds the
 * no-op implementations from [FossPushModule] instead — only one of the two
 * modules is ever on the classpath, so Hilt sees a single binding.
 */
@Module
@InstallIn(SingletonComponent::class)
object FcmPushModule {

    @Provides
    fun provideFcmTokenSource(impl: FirebaseFcmTokenSource): FcmTokenSource = impl

    @Provides
    fun provideFcmAvailability(impl: FirebaseFcmAvailability): FcmAvailability = impl
}
