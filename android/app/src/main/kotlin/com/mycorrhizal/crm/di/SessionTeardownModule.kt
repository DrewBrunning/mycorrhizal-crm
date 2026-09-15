package com.mycorrhizal.crm.di

import com.mycorrhizal.crm.data.auth.CurrentSessionRevoker
import com.mycorrhizal.crm.data.session.SessionTeardown
import com.mycorrhizal.crm.feature.tracking.DeviceRegistrationManager
import dagger.Binds
import dagger.Module
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Issue #957: the real [SessionTeardown] — core:data only knows the
 * interface (see its doc comment for why), so this is where its two pieces
 * actually meet: FCM device deregistration (feature:tracking) and the
 * server-side session revoke (core:data's [CurrentSessionRevoker]). Lives in
 * :app because that is the only module that depends on both.
 *
 * Both steps run and are independently best-effort — one failing must not
 * skip or fail the other, and neither may throw out of [beforeClear] and
 * block [com.mycorrhizal.crm.data.session.DefaultSessionManager.clearSession]'s
 * local clear (that class also wraps this call in `runCatching`, but this
 * class does not rely on it: a request that never reaches the *second* step
 * because the first one threw would be a self-inflicted regression of the
 * same shape as the bug #957 exists to fix).
 */
@Singleton
class AppSessionTeardown @Inject constructor(
    private val deviceRegistration: DeviceRegistrationManager,
    private val sessionRevoker: CurrentSessionRevoker,
) : SessionTeardown {
    override suspend fun beforeClear() {
        runCatching { deviceRegistration.delete() }
        runCatching { sessionRevoker.revoke() }
    }
}

@Module
@InstallIn(SingletonComponent::class)
abstract class SessionTeardownModule {
    @Binds
    abstract fun bindSessionTeardown(impl: AppSessionTeardown): SessionTeardown
}
