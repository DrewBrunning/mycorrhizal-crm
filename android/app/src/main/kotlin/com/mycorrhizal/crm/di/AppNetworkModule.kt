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
 * interceptor chain: Auth → BaseUrl → Logging (debug) → Retry. The token
 * and base-URL providers come from the SessionManager (wired in core:data).
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
