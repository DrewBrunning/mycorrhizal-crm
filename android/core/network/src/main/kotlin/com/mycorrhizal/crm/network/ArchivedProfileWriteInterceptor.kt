package com.mycorrhizal.crm.network

import okhttp3.Interceptor
import okhttp3.Response
import java.io.IOException

/**
 * Whether the active server profile is a read-only archive (ADR 0028 Decision
 * 3: a `Local` profile whose data was moved to a server). Read synchronously
 * from an in-memory cache, like the token and base-URL providers.
 */
fun interface ArchivedProfileProvider {
    fun isActiveProfileArchived(): Boolean
}

/** Thrown for a write attempted against an archived profile; never reaches a server. */
class ArchivedProfileWriteException(val method: String) :
    IOException("Write blocked: the active profile is a read-only archive ($method)")

/**
 * ADR 0028 Decision 3: an archived profile "can still be browsed, and every
 * write is blocked by an interceptor that fails writes on archived profiles
 * with a clear message." Any method other than GET/HEAD/OPTIONS is failed
 * before it leaves the process ([toApiError] maps it to [ApiError.ArchivedProfile]).
 *
 * Runs FIRST in the chain so no other interceptor (retry, auth) sees a write it
 * is about to refuse. [ArchivedProfileProvider] is consulted per request, so a
 * profile switch takes effect on the very next call.
 */
class ArchivedProfileWriteInterceptor(
    private val provider: ArchivedProfileProvider,
) : Interceptor {
    override fun intercept(chain: Interceptor.Chain): Response {
        val request = chain.request()
        if (provider.isActiveProfileArchived() && request.method !in READ_ONLY_METHODS) {
            throw ArchivedProfileWriteException(request.method)
        }
        return chain.proceed(request)
    }

    private companion object {
        val READ_ONLY_METHODS = setOf("GET", "HEAD", "OPTIONS")
    }
}
