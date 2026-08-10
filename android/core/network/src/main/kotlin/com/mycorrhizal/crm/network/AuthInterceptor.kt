package com.mycorrhizal.crm.network

import okhttp3.HttpUrl.Companion.toHttpUrlOrNull
import okhttp3.Interceptor
import okhttp3.Response

/** Supplies the current bearer token synchronously (read from an in-memory cache). */
fun interface TokenProvider {
    fun bearerToken(): String?
}

/**
 * Adds `Authorization: Bearer <token>` to every request when a token is
 * present. Requests without a stored token pass through untouched (the login
 * call itself never carries the header).
 *
 * The token is only attached when the request targets the configured server
 * origin ([BaseUrlProvider]). This is defense in depth: every API call is
 * built against a placeholder origin and rewritten by [BaseUrlInterceptor],
 * but if a future code path ever reuses this client for a different host
 * (image loading, deep links), the JWT is not silently leaked to it.
 */
class AuthInterceptor(
    private val tokenProvider: TokenProvider,
    private val baseUrlProvider: BaseUrlProvider,
) : Interceptor {
    override fun intercept(chain: Interceptor.Chain): Response {
        val token = tokenProvider.bearerToken()
        val request = chain.request()
        if (token.isNullOrBlank() || request.header("Authorization") != null) {
            return chain.proceed(request)
        }
        val base = baseUrlProvider.baseUrl().toHttpUrlOrNull()
        val sameHost = base != null && request.url.host == base.host
        if (!sameHost) {
            return chain.proceed(request)
        }
        return chain.proceed(
            request.newBuilder().header("Authorization", "Bearer $token").build(),
        )
    }
}
