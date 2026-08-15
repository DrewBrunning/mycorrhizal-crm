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
