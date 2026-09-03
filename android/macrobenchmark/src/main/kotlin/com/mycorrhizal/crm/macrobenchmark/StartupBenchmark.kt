package com.mycorrhizal.crm.macrobenchmark

import androidx.benchmark.macro.CompilationMode
import androidx.benchmark.macro.StartupMode
import androidx.benchmark.macro.StartupTimingMetric
import androidx.benchmark.macro.junit4.MacrobenchmarkRule
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Issue #263: cold / warm / hot startup of the app, to first frame
 * ([StartupTimingMetric] → `timeToInitialDisplayMs`).
 *
 * Measures the launch path up to the first rendered frame — the splash screen
 * and then either the login screen or, if a session is already persisted, the
 * dashboard. No backend needed: this is the process-start + first-frame cost,
 * independent of what the first screen then loads.
 *
 * `CompilationMode.DEFAULT` matches how a Play-installed app runs (no Baseline
 * Profile is shipped yet — that is a separate follow-up). CI records the number
 * as a trend; it never asserts a threshold (emulator variance).
 */
@RunWith(AndroidJUnit4::class)
class StartupBenchmark {

    @get:Rule
    val benchmarkRule = MacrobenchmarkRule()

    @Test
    fun coldStartup() = measure(StartupMode.COLD)

    @Test
    fun warmStartup() = measure(StartupMode.WARM)

    @Test
    fun hotStartup() = measure(StartupMode.HOT)

    private fun measure(startupMode: StartupMode) = benchmarkRule.measureRepeated(
        packageName = BenchmarkConfig.TARGET_PACKAGE,
        metrics = listOf(StartupTimingMetric()),
        compilationMode = CompilationMode.DEFAULT,
        startupMode = startupMode,
        iterations = BenchmarkConfig.STARTUP_ITERATIONS,
        setupBlock = { pressHome() },
    ) {
        startActivityAndWait()
    }
}
