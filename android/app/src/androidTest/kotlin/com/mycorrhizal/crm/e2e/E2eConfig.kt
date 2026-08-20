package com.mycorrhizal.crm.e2e

import androidx.test.platform.app.InstrumentationRegistry

/**
 * Shared constants for the instrumented E2E suite (issue #238).
 *
 * The suite drives the real app (MainActivity + the real Hilt graph) against
 * the same docker-compose.test.yml backend the web Playwright suite uses
 * (.github/workflows/e2e-tests.yml). The URL is overridable per run with the
 * instrumentation arg `-e serverUrl http://<host>:7300` so a developer can
 * point the suite at a LAN-reachable backend without touching source.
 *
 * Reachability:
 *  - Emulator: `http://10.0.2.2:7300` is the host machine's loopback, on the
 *    debug build's cleartext allowlist
 *    (src/debug/res/xml/network_security_config.xml).
 *  - Physical device (Pixel 8a): `adb reverse tcp:7300 tcp:7300`, then
 *    `http://127.0.0.1:7300` (also on the allowlist). See README-developer.md.
 */
object E2eConfig {
    private val args = InstrumentationRegistry.getArguments()

    /** The backend origin the suite logs into. */
    val serverUrl: String = args.getString("serverUrl")
        ?.trim()
        ?.takeIf { it.isNotBlank() }
        ?: "http://10.0.2.2:7300"

    val apiBaseUrl: String get() = "$serverUrl/api/v1"

    /** The shared seed account, registered idempotently before every test. */
    const val SEED_USERNAME = "e2euser"
    const val SEED_EMAIL = "e2euser@example.com"
    const val SEED_PASSWORD = "E2eTestPassword123!"

    /** Namespace every contact this suite creates is prefixed with — swept up
     *  at the start of every run so a crashed run's leftovers can't bleed into
     *  the next one (the web suite's E2E_CONTACT_PREFIX equivalent). */
    const val TEST_CONTACT_PREFIX = "E2E"
}
