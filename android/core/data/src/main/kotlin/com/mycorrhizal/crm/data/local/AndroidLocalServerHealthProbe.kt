package com.mycorrhizal.crm.data.local

import com.mycorrhizal.crm.network.LOCAL_SERVER_SENTINEL_URL
import com.mycorrhizal.crm.network.LocalSocketPathProvider
import com.mycorrhizal.crm.network.ProfileAwareDns
import com.mycorrhizal.crm.network.ProfileAwareSocketFactory
import okhttp3.OkHttpClient
import okhttp3.Request
import java.util.concurrent.TimeUnit

/**
 * GET /health over the embedded server's Unix socket. Uses the same
 * profile-aware transport the app's own client does, but pinned to one socket
 * path so the host can probe before any session exists. # pragma: no cover —
 * a real socket probe; the host's readiness logic is covered via a fake.
 */
class AndroidLocalServerHealthProbe : LocalServerHealthProbe {

    // Keyed by socket path: the poll loop calls this repeatedly, and building a
    // fresh client (and connection pool) on every attempt would be wasteful.
    // The path is stable for the process's lifetime.
    private val clients = mutableMapOf<String, OkHttpClient>()

    @Suppress("TooGenericExceptionCaught") // process/socket boundary: any failure is "not healthy"
    override suspend fun isHealthy(socketPath: String): Boolean {
        val pinnedClient = synchronized(clients) {
            clients.getOrPut(socketPath) {
                val provider = LocalSocketPathProvider { socketPath }
                OkHttpClient.Builder()
                    .socketFactory(ProfileAwareSocketFactory(provider))
                    .dns(ProfileAwareDns(provider))
                    .connectTimeout(2, TimeUnit.SECONDS)
                    .readTimeout(5, TimeUnit.SECONDS)
                    .build()
            }
        }
        return runCatching {
            pinnedClient.newCall(
                Request.Builder().url("$LOCAL_SERVER_SENTINEL_URL/health").build(),
            ).execute().use { it.isSuccessful }
        }.getOrDefault(false)
    }
}
