package com.mycorrhizal.crm.feature.tracking

import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.domain.repository.ActivityRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import io.mockk.mockk
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/**
 * [QuickCaptureOverlay] is an instance class (not a singleton `object`) so
 * the View it retains while shown is scoped to its owner's lifetime, not the
 * process's — this pins down that instances are genuinely independent state,
 * which a shared static holder would not have been.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
class QuickCaptureOverlayTest {

    private val context = ApplicationProvider.getApplicationContext<android.content.Context>()
    private val contactRepository = mockk<ContactRepository>(relaxed = true)
    private val activityRepository = mockk<ActivityRepository>(relaxed = true)

    @Test
    fun `show marks the overlay as showing`() {
        val overlay = QuickCaptureOverlay(contactRepository, activityRepository)
        overlay.show(context)
        assertTrue(overlay.isShowingForTest())
    }

    @Test
    fun `dismiss clears the overlay`() {
        val overlay = QuickCaptureOverlay(contactRepository, activityRepository)
        overlay.show(context)
        overlay.dismiss()
        assertFalse(overlay.isShowingForTest())
    }

    @Test
    fun `dismissing one instance does not affect another`() {
        // Two independent owners (e.g. two CallDetectionService instances)
        // must not share overlay state the way a singleton `object` would.
        val first = QuickCaptureOverlay(contactRepository, activityRepository)
        val second = QuickCaptureOverlay(contactRepository, activityRepository)
        first.show(context)
        second.show(context)

        second.dismiss()

        assertTrue(first.isShowingForTest())
        assertFalse(second.isShowingForTest())
    }
}
