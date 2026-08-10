package com.mycorrhizal.crm.network

import okhttp3.HttpUrl.Companion.toHttpUrlOrNull
import okhttp3.Interceptor
import okhttp3.Response

/** Supplies the configured server origin synchronously. */
fun interface BaseUrlProvider {
    fun baseUrl(): String
}

/**
 * Rewrites the scheme/host/port of every request onto the user-configured
 * server origin. API calls are built against a placeholder origin so they
 * stay testable; this interceptor is what actually points them at the real
 * backend (ticket §2.2, slot 2).
 */
class BaseUrlInterceptor(private val baseUrlProvider: BaseUrlProvider) : Interceptor {
    override fun intercept(chain: Interceptor.Chain): Response {
        val base = baseUrlProvider.baseUrl()
        if (base.isBlank()) return chain.proceed(chain.request())

        val baseUrl = base.toHttpUrlOrNull() ?: return chain.proceed(chain.request())
        val url = chain.request().url.newBuilder()
            .scheme(baseUrl.scheme)
            .host(baseUrl.host)
            .port(baseUrl.port)
            .build()
        return chain.proceed(chain.request().newBuilder().url(url).build())
    }
}
