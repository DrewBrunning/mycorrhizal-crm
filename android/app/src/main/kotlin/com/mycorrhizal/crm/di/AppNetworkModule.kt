package com.mycorrhizal.crm.di

import com.mycorrhizal.crm.BuildConfig
import com.mycorrhizal.crm.data.local.LocalServerHost
import com.mycorrhizal.crm.network.ArchivedProfileProvider
import com.mycorrhizal.crm.data.attach.AttachRemoteApiFactory
import com.mycorrhizal.crm.data.attach.DefaultAttachRemoteApiFactory
import com.mycorrhizal.crm.network.BaseUrlProvider
import com.mycorrhizal.crm.network.ClientVersionProvider
import com.mycorrhizal.crm.network.LOCAL_SERVER_SENTINEL_URL
import com.mycorrhizal.crm.network.LocalSocketPathProvider
import com.mycorrhizal.crm.network.NetworkFactory
import com.mycorrhizal.crm.network.SessionExpiryInterceptor
import com.mycorrhizal.crm.network.SessionExpiryNotifier
import com.mycorrhizal.crm.network.TokenProvider
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import okhttp3.OkHttpClient
import javax.inject.Singleton

/**
 * Provides the application-scoped OkHttpClient with the ticket §2.2
 * interceptor chain. The ACTUAL order is BaseUrl → Auth → Retry (then Logging
 * in debug) — see NetworkFactory; BaseUrl must run before Auth so Auth's host
 * check sees the rewritten URL. The token and base-URL providers come from the
 * SessionManager (wired in core:data). This exact ordering is load-bearing for
 * M5 §3.1 (Coil reuses this client for photo URLs).
 *
 * Issue #678: the SessionExpiryInterceptor (registered after Retry, before the
 * debug logging interceptor) watches every response for a 401 and signals
 * SessionExpiryNotifier; the session wiring clears the session on that signal
 * so the app lands back on the auth flow.
 */
@Module
@InstallIn(SingletonComponent::class)
object AppNetworkModule {
    @Provides
    @Singleton
    fun provideOkHttpClient(
        tokenProvider: TokenProvider,
        baseUrlProvider: BaseUrlProvider,
        sessionExpiryNotifier: SessionExpiryNotifier,
        localServerHost: LocalServerHost,
        archivedProfileProvider: ArchivedProfileProvider,
    ): OkHttpClient = NetworkFactory.okHttpClient(
        tokenProvider = tokenProvider,
        baseUrlProvider = baseUrlProvider,
        debug = BuildConfig.DEBUG,
        // ADR 0028 Decision 3: writes on a read-only archived profile fail
        // before leaving the process.
        archivedProfileProvider = archivedProfileProvider,
        sessionExpiryInterceptor = SessionExpiryInterceptor(sessionExpiryNotifier, baseUrlProvider),
        // ADR 0028 Decision 2: route `Local`-profile traffic over the embedded
        // server's Unix socket through the one shared client. Null when a Remote
        // profile is active or the embedded server is not running, which keeps
        // the ordinary network path.
        localSocketPathProvider = LocalSocketPathProvider {
            if (baseUrlProvider.baseUrl() == LOCAL_SERVER_SENTINEL_URL) {
                localServerHost.socketPathIfRunning()
            } else {
                null
            }
        },
        // Issue #692: the app advertises its versionName on every API request
        // so the server can log it and enforce its MIN_CLIENT_VERSION floor at
        // authentication. BuildConfig is readable here because this module is
        // in :app; the interceptor keeps the header off non-API hosts.
        clientVersionProvider = ClientVersionProvider { BuildConfig.VERSION_NAME },
    ).newBuilder()
        // Appended last, so it sees the rewritten (sentinel) URL and runs before
        // the socket transport: the first Local request starts the embedded
        // server, and every later request is a fast no-op.
        .addInterceptor(LocalServerWakeInterceptor(localServerHost))
        .build()

    /** ADR 0028 Decision 3 / issue #1265: the attach wizard's client to the chosen Remote server. */
    @Provides
    fun provideAttachRemoteApiFactory(): AttachRemoteApiFactory =
        DefaultAttachRemoteApiFactory(
            clientVersionProvider = ClientVersionProvider { BuildConfig.VERSION_NAME },
            debug = BuildConfig.DEBUG,
        )
}
