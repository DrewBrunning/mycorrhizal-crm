package com.mycorrhizal.crm.feature.tracking

import android.os.Handler
import android.os.Looper

/**
 * #201 (WCAG 2.2.1 Timing Adjustable): the auto-dismiss timer for the
 * quick-capture overlay. [arm] schedules [onDismiss] after the display window
 * so an ignored overlay does not sit on screen forever; [cancel] — wired to
 * the sheet's first focus/keystroke — removes it so the limit never fires
 * while the user is mid-form. Because the timer only fires when the sheet was
 * never engaged, no typed content is ever silently dropped.
 *
 * Extracted from [QuickCaptureOverlay] so the arm/cancel contract is
 * unit-testable without the overlay's Compose window, which needs a
 * ViewTreeLifecycleOwner that plain Robolectric does not provide.
 */
internal class QuickCaptureAutoDismiss(
    private val onDismiss: () -> Unit,
) {
    private val handler = Handler(Looper.getMainLooper())

    fun arm() {
        handler.removeCallbacksAndMessages(null)
        handler.postDelayed({ onDismiss() }, DISPLAY_MS)
    }

    fun cancel() {
        handler.removeCallbacksAndMessages(null)
    }

    private companion object {
        const val DISPLAY_MS = 30_000L
    }
}
