package com.mycorrhizal.crm.network

import okhttp3.HttpUrl.Companion.toHttpUrlOrNull
import okhttp3.Interceptor
import okhttp3.Response

/**
 * Observes a 403 on an authenticated request to the configured server and
 * signals [ForbiddenNotifier] (issue #1826). Purely observational, like
 * [SessionExpiryInterceptor]: the response is returned untouched, and a 401 is
 * deliberately NOT handled here (that is the session-expired flow).
 *
 * The signal only fires for the configured server origin and only for a
 * request that carried a bearer token: the app's OkHttp client is also Coil's
 * photo client, so an off-server 403 must not trigger a profile re-fetch, and a
 * 403 on an unauthenticated call has no role to refresh.
 */
class ForbiddenInterceptor(
    private val notifier: ForbiddenNotifier,
    private val baseUrlProvider: BaseUrlProvider,
) : Interceptor {

    override fun intercept(chain: Interceptor.Chain): Response {
        val request = chain.request()
        val response = chain.proceed(request)
        val base = baseUrlProvider.baseUrl().toHttpUrlOrNull()
        val authenticated = response.request.header("Authorization")
            ?.startsWith("Bearer ", ignoreCase = true) == true
        if (response.code == 403 && base != null && request.url.host == base.host && authenticated) {
            notifier.onForbidden()
        }
        return response
    }
}
