package com.mycorrhizal.crm.macrobenchmark

import androidx.test.uiautomator.By
import androidx.test.uiautomator.Direction
import androidx.test.uiautomator.UiDevice
import androidx.test.uiautomator.Until

/**
 * Scrolls the dashboard feed end-to-end and back so [DashboardRenderBenchmark]'s
 * `FrameTimingMetric` captures the recomposition/scroll cost of the composite
 * that a single `/dashboard` call populates (issue #263).
 */
internal object DashboardActions {

    private const val SCROLL_STEPS = 3

    fun scrollFeed(device: UiDevice) {
        // The dashboard is a single LazyColumn — the one scrollable container on
        // screen once the app-bar title is up. `testTagsAsResourceId` is not
        // enabled app-wide, so match on the scrollable role rather than a res-id.
        val feed = checkNotNull(
            device.wait(Until.findObject(By.scrollable(true)), 5_000),
        ) { "dashboard scroll container not found" }

        // Keep gestures clear of the system bars / app bar.
        feed.setGestureMargin(device.displayWidth / 5)

        repeat(SCROLL_STEPS) {
            feed.scroll(Direction.DOWN, 1f)
            device.waitForIdle()
        }
        repeat(SCROLL_STEPS) {
            feed.scroll(Direction.UP, 1f)
            device.waitForIdle()
        }
    }
}
