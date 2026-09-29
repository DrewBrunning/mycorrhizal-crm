package com.mycorrhizal.crm.di

import com.mycorrhizal.crm.data.local.LocalServerHost
import com.mycorrhizal.crm.network.LOCAL_SERVER_SENTINEL_HOST
import kotlinx.coroutines.runBlocking
import okhttp3.Interceptor
import okhttp3.Response
import java.io.IOException

/**
 * ADR 0028 Decision 2 / issue #1262: starts the embedded server lazily from the
 * first request or worker that needs the `Local` profile. Every `Local`-profile
 * request is addressed to the sentinel host after [com.mycorrhizal.crm.network.BaseUrlInterceptor]
 * rewrites it, and this interceptor — which runs after that rewrite — blocks
 * the request on [LocalServerHost.ensureStarted] before letting it proceed to
 * the transport. This is why the ~30 repositories, the direct `ApiClient` users
 * and the WorkManager workers need no local-mode code of their own: the one
 * shared client guarantees the server is up before any of them can send.
 *
 * The blocking call runs on OkHttp's dispatcher/connection thread (never the
 * main thread — every `ApiClient` call is on `Dispatchers.IO`), so it cannot
 * block a frame. It is a fast no-op once the server is running.
 *
 * A failure to start becomes an [IOException] on the request, i.e. an ordinary
 * network-shaped failure: the same path a dropped socket takes (ADR 0028,
 * "process death mid-write ... the client must treat a dropped socket like a
 * network error").
 */
class LocalServerWakeInterceptor(
    private val localServerHost: LocalServerHost,
) : Interceptor {
    override fun intercept(chain: Interceptor.Chain): Response {
        val request = chain.request()
        if (request.url.host == LOCAL_SERVER_SENTINEL_HOST) {
            runBlocking { localServerHost.ensureStarted() }.getOrElse { error ->
                throw IOException("embedded local server is unavailable", error)
            }
        }
        return chain.proceed(request)
    }
}
