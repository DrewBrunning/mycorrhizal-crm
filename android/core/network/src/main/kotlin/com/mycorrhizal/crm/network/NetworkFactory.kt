package com.mycorrhizal.crm.network

import com.mycorrhizal.crm.model.MoshiProvider
import com.squareup.moshi.Moshi
import okhttp3.OkHttpClient
import java.util.concurrent.TimeUnit

/**
 * Builds the OkHttp stack and Moshi instance. Interceptor order follows the
 * ticket (§2.2): Auth → BaseUrl → Logging → Retry, all appended to the
 * client so OkHttp runs them in registration order.
 */
object NetworkFactory {
    const val CONNECT_TIMEOUT_SECONDS = 30L
    const val READ_TIMEOUT_SECONDS = 30L

    fun moshi(): Moshi = MoshiProvider.get()

    fun okHttpClient(
        tokenProvider: TokenProvider,
        baseUrlProvider: BaseUrlProvider,
        debug: Boolean = false,
    ): OkHttpClient {
        val builder = OkHttpClient.Builder()
            .addInterceptor(AuthInterceptor(tokenProvider))
            .addInterceptor(BaseUrlInterceptor(baseUrlProvider))
            .addInterceptor(RetryInterceptor())
            .connectTimeout(CONNECT_TIMEOUT_SECONDS, TimeUnit.SECONDS)
            .readTimeout(READ_TIMEOUT_SECONDS, TimeUnit.SECONDS)

        if (debug) {
            builder.addInterceptor(
                okhttp3.logging.HttpLoggingInterceptor().apply {
                    level = okhttp3.logging.HttpLoggingInterceptor.Level.BASIC
                },
            )
        }
        return builder.build()
    }
}
