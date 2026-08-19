package com.mycorrhizal.crm.feature.tracking

import android.os.Looper
import org.junit.Assert.assertEquals
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.Shadows.shadowOf
import org.robolectric.annotation.Config
import java.time.Duration

/**
 * #201 (WCAG 2.2.1 Timing Adjustable): the quick-capture overlay must dismiss
 * itself after the display window so an ignored sheet does not sit on screen
 * forever, but the timer must be cancellable so a user who has engaged the
 * form is never timed out mid-save or mid-typing. `idleFor` advances the
 * paused Robolectric looper without touching a real clock or any Compose
 * window, which makes the arm/cancel contract directly testable.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
class QuickCaptureAutoDismissTest {

    private val displayWindow = Duration.ofSeconds(30)

    @Test
    fun `an armed timer dismisses once the window elapses untouched`() {
        var dismissed = 0
        val timer = QuickCaptureAutoDismiss(onDismiss = { dismissed++ })

        timer.arm()
        shadowOf(Looper.getMainLooper()).idleFor(displayWindow)

        assertEquals(1, dismissed)
    }

    @Test
    fun `cancelling before the window prevents the dismiss`() {
        var dismissed = 0
        val timer = QuickCaptureAutoDismiss(onDismiss = { dismissed++ })

        timer.arm()
        timer.cancel()
        shadowOf(Looper.getMainLooper()).idleFor(displayWindow)

        assertEquals(0, dismissed)
    }

    @Test
    fun `re-arming replaces the pending dismiss instead of stacking`() {
        var dismissed = 0
        val timer = QuickCaptureAutoDismiss(onDismiss = { dismissed++ })

        timer.arm()
        shadowOf(Looper.getMainLooper()).idleFor(Duration.ofSeconds(10))
        timer.arm()
        shadowOf(Looper.getMainLooper()).idleFor(Duration.ofSeconds(29))

        // Second arm re-posted from ~t+10s, so at t+39s the window is not up.
        assertEquals(0, dismissed)
    }
}
