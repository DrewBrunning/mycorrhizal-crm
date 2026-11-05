package com.mycorrhizal.crm.ui

import androidx.compose.runtime.staticCompositionLocalOf

/**
 * The configured server origin (scheme://host:port), provided at the app root
 * once the session knows it. ContactAvatar (M5 §3.1) resolves the backend's
 * relative profile-photo paths against it so the resulting absolute URL is
 * unique per server — which is also the Coil disk-cache key, so switching
 * servers can't serve one instance's cached avatar to another. Blank in
 * previews/tests, where the avatar falls back to the placeholder-origin trick
 * (the interceptors still rewrite it).
 */
val LocalServerUrl = staticCompositionLocalOf { "" }

/**
 * Resolves a server-relative image path (profile photos, ADR 0040 owner
 * images) for Coil: a `/`-prefixed path is joined to [serverOrigin] when known,
 * else the placeholder origin — in both cases the shared OkHttp stack's
 * BaseUrlInterceptor points the request at the configured server and the
 * AuthInterceptor attaches the JWT. Resolving to the REAL origin keeps the
 * Coil disk-cache key per-server. Absolute URLs and `data:` URIs pass through
 * untouched; blank input resolves to null.
 */
fun resolveServerUri(uri: String?, serverOrigin: String): String? {
    if (uri == null) return null
    val trimmed = uri.trim()
    if (trimmed.isEmpty()) return null
    return if (trimmed.startsWith("/")) {
        val origin = serverOrigin.trim().trimEnd('/').takeIf { it.isNotBlank() }
            ?: com.mycorrhizal.crm.network.ApiClient.PLACEHOLDER_ORIGIN
        origin + trimmed
    } else {
        trimmed
    }
}
