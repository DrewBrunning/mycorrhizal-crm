package com.mycorrhizal.crm.macrobenchmark

import androidx.test.platform.app.InstrumentationRegistry

/**
 * Shared constants for the macrobenchmark suite (issue #263).
 *
 * The startup scenarios need only [TARGET_PACKAGE]. The dashboard scenario also
 * logs in against the same `docker-compose.test.yml` backend the instrumented
 * E2E suite uses, so it reuses that suite's seed account and its `serverUrl`
 * instrumentation-arg override (see `E2eConfig` in `app/src/androidTest`).
 */
internal object BenchmarkConfig {

    /** The app under test — `applicationId` from the application convention plugin. */
    const val TARGET_PACKAGE = "com.mycorrhizal.crm"

    /** The app-bar title shown once the dashboard (home) route is on screen. */
    const val DASHBOARD_TITLE = "Dashboard"

    /** Iterations per startup scenario. Small: the CI signal is the trend, not
     *  a tight confidence interval, and the emulator job's budget is shared. */
    const val STARTUP_ITERATIONS = 8

    /** Iterations for the dashboard frame-timing scenario (each one re-scrolls
     *  the whole feed, so it is heavier than a startup iteration). */
    const val DASHBOARD_ITERATIONS = 6

    // --- seed account (mirrors app/src/androidTest E2eConfig) -----------------

    const val SEED_USERNAME = "e2euser"
    const val SEED_EMAIL = "e2euser@example.com"
    const val SEED_PASSWORD = "E2eTestPassword123!"

    /**
     * Backend origin the dashboard scenario logs into. Overridable per run with
     * `-Pandroid.testInstrumentationRunnerArguments.serverUrl=http://<host>:7300`;
     * defaults to the emulator's host-loopback alias (the CI job also does
     * `adb reverse tcp:7300 tcp:7300`, so 127.0.0.1 works too when passed).
     */
    val serverUrl: String
        get() = InstrumentationRegistry.getArguments().getString("serverUrl")
            ?.trim()
            ?.takeIf { it.isNotEmpty() }
            ?: "http://10.0.2.2:7300"
}
