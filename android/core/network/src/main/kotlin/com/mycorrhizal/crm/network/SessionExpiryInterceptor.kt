package com.mycorrhizal.crm.network

import okhttp3.Interceptor
import okhttp3.Response

/**
 * Detects a 401 on any API response and signals [SessionExpiryNotifier] so the
 * session can be cleared and the app can land on the auth flow (issue #678).
 * Deliberately the outermost concern in the interceptor chain: the request is
 * passed through untouched, and the response is returned untouched — this
 * interceptor only *observes* the response code. A stale/expired bearer token
 * must never leave the user staring at a stuck loading state or a screen that
 * silently half-rendered with no session.
 *
 * Clearing on 401 is idempotent: a login attempt with wrong credentials also
 * 401s, but clearing an already-empty session is a no-op, so no special case is
 * needed for the login endpoint.
 */
class SessionExpiryInterceptor(
    private val notifier: SessionExpiryNotifier,
) : Interceptor {

    override fun intercept(chain: Interceptor.Chain): Response {
        val response = chain.proceed(chain.request())
        if (response.code == 401) {
            notifier.onSessionExpired()
        }
        return response
    }
}
