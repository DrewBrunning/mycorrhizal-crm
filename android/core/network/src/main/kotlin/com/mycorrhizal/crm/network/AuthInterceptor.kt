package com.mycorrhizal.crm.network

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
 */
class AuthInterceptor(private val tokenProvider: TokenProvider) : Interceptor {
    override fun intercept(chain: Interceptor.Chain): Response {
        val token = tokenProvider.bearerToken()
        val request = chain.request()
        if (token.isNullOrBlank() || request.header("Authorization") != null) {
            return chain.proceed(request)
        }
        return chain.proceed(
            request.newBuilder().header("Authorization", "Bearer $token").build(),
        )
    }
}
