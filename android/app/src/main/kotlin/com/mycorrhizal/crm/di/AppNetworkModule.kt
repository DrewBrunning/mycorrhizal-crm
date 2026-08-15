package com.mycorrhizal.crm.di

import com.mycorrhizal.crm.BuildConfig
import com.mycorrhizal.crm.network.BaseUrlProvider
import com.mycorrhizal.crm.network.NetworkFactory
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
 */
@Module
@InstallIn(SingletonComponent::class)
object AppNetworkModule {
    @Provides
    @Singleton
    fun provideOkHttpClient(
        tokenProvider: TokenProvider,
        baseUrlProvider: BaseUrlProvider,
    ): OkHttpClient = NetworkFactory.okHttpClient(
        tokenProvider = tokenProvider,
        baseUrlProvider = baseUrlProvider,
        debug = BuildConfig.DEBUG,
    )
}
