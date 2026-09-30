package com.mycorrhizal.crm.e2e

import android.os.Looper
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.runner.lifecycle.ActivityLifecycleMonitorRegistry
import androidx.test.runner.lifecycle.Stage
import com.mycorrhizal.crm.MainActivity

/**
 * Issue #1372: the one teardown every intent-delivery E2E class shares
 * ([DeepLinkE2eTest], [ShareToCrmE2eTest], [CrossTaskIntentE2eTest]).
 *
 * A test that delivers an intent (deep link / share) leaves MainActivity PAUSED behind
 * the instrumentation's own EmptyActivity task, and `ActivityScenarioRule`'s teardown
 * (`ActivityScenario.close`) then blocks until it gives up with `Activity never becomes
 * requested state "[DESTROYED]" (last lifecycle transition = "PAUSED")` — failing a test
 * whose body passed. Finishing the activity ourselves, *before* the rule's `after` runs,
 * leaves `close()` nothing to wait for.
 *
 * Every function here is safe to call from the main thread or a test thread (the
 * lifecycle monitor must be read on the main thread, and `runOnMainSync` throws when
 * already on it — the #1372 `CrossTaskIntentE2eTest` failure).
 */
internal object E2eActivities {

    private val instrumentation get() = InstrumentationRegistry.getInstrumentation()

    /** Runs [block] on the main thread and returns its result, without re-entering `runOnMainSync`. */
    fun <T> onMain(block: () -> T): T {
        if (Looper.myLooper() == Looper.getMainLooper()) return block()
        var result: Result<T>? = null
        instrumentation.runOnMainSync { result = runCatching(block) }
        return result!!.getOrThrow()
    }

    /** Every MainActivity the lifecycle monitor still considers alive. */
    fun mainActivities(): List<MainActivity> = onMain {
        val monitor = ActivityLifecycleMonitorRegistry.getInstance()
        Stage.values()
            .filter { it != Stage.DESTROYED }
            .flatMap { monitor.getActivitiesInStage(it).filterIsInstance<MainActivity>() }
    }

    /**
     * Finishes every live MainActivity directly and polls (bounded, never throws) until
     * they are gone, so a following `ActivityScenario.close()` is already satisfied.
     */
    fun finishAllMainActivities(timeoutMs: Long = 10_000) {
        onMain { mainActivities().forEach { it.finishAndRemoveTask() } }
        val deadline = System.currentTimeMillis() + timeoutMs
        while (System.currentTimeMillis() < deadline && mainActivities().isNotEmpty()) Thread.sleep(200)
    }
}
