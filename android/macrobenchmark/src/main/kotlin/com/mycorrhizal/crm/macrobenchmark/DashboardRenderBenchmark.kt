package com.mycorrhizal.crm.macrobenchmark

import androidx.benchmark.macro.CompilationMode
import androidx.benchmark.macro.FrameTimingMetric
import androidx.benchmark.macro.StartupMode
import androidx.benchmark.macro.junit4.MacrobenchmarkRule
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Issue #263: dashboard render/scroll frame timing ([FrameTimingMetric] →
 * `frameDurationCpuMs` / `frameOverrunMs` percentiles).
 *
 * The dashboard is the app's main surface — one `/dashboard` call fans out into
 * favourites, overdue cadences, reach-outs, birthdays, reminders and random
 * contacts, all rendered in a single LazyColumn. This scenario logs in against
 * the `docker-compose.test.yml` backend (same one the instrumented E2E suite
 * uses), waits for the dashboard, then scrolls the feed top-to-bottom and back
 * while frames are measured, so a recomposition regression in that composite
 * shows up as a jank trend.
 *
 * `setupBlock` re-launches the activity each iteration; the persisted session
 * means only the first iteration actually walks the login UI.
 */
@RunWith(AndroidJUnit4::class)
class DashboardRenderBenchmark {

    @get:Rule
    val benchmarkRule = MacrobenchmarkRule()

    @Test
    fun scrollDashboard() {
        SeedBackend.ensureSeedUser()

        benchmarkRule.measureRepeated(
            packageName = BenchmarkConfig.TARGET_PACKAGE,
            metrics = listOf(FrameTimingMetric()),
            compilationMode = CompilationMode.DEFAULT,
            startupMode = StartupMode.WARM,
            iterations = BenchmarkConfig.DASHBOARD_ITERATIONS,
            setupBlock = {
                pressHome()
                startActivityAndWait()
                LoginActions.ensureOnDashboard(device)
            },
        ) {
            DashboardActions.scrollFeed(device)
        }
    }
}
