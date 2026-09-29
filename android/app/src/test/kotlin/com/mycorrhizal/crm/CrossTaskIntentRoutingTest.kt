package com.mycorrhizal.crm

import android.app.Application
import android.content.Intent
import android.net.Uri
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

// Issue #1343: per-PR (JVM) coverage of the cross-task facts the instrumented
// CrossTaskIntentE2eTest proves on a device. An external start (browser, another app,
// notification, `adb shell am start`) reaches MainActivity with FLAG_ACTIVITY_NEW_TASK
// (and often RESET_TASK_IF_NEEDED / CLEAR_TOP) — onCreate when cold, onNewIntent when
// warm; both funnel through consumeLaunchIntent(), whose gate is
// shouldHandleLaunchIntent(savedStateIsNull, flags). None of those flags may suppress
// handling; only a recreate (saved state) or a Recents reopen may.
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
class CrossTaskIntentRoutingTest {

    private val externalFlags = listOf(
        Intent.FLAG_ACTIVITY_NEW_TASK,
        Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_RESET_TASK_IF_NEEDED,
        Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP,
        Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_SINGLE_TOP,
    )

    @Test
    fun `cross-task launch flags never suppress handling`() {
        externalFlags.forEach { flags ->
            // cold: onCreate with no saved state; warm: onNewIntent always passes savedStateIsNull=true
            assertTrue(shouldHandleLaunchIntent(savedStateIsNull = true, flags = flags))
        }
    }

    @Test
    fun `a cross-task start that is a Recents reopen or a recreate is still a replay`() {
        externalFlags.forEach { flags ->
            assertFalse(shouldHandleLaunchIntent(true, flags or Intent.FLAG_ACTIVITY_LAUNCHED_FROM_HISTORY))
            assertFalse(shouldHandleLaunchIntent(false, flags))
        }
    }

    @Test
    fun `a NEW_TASK VIEW intent yields its link and routes strictly`() {
        val view = Intent(Intent.ACTION_VIEW, Uri.parse("mycorrhizal://contacts/7"))
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        assertEquals("contacts/7", deepLinkRoute(deepLinkUri(view)))

        // Strict parsing is unaffected by the delivery path: rejected links stay rejected.
        listOf("mycorrhizal://settings", "mycorrhizal://contacts/42/edit").forEach {
            val rejected = Intent(Intent.ACTION_VIEW, Uri.parse(it)).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            assertNull(deepLinkRoute(deepLinkUri(rejected)))
        }
    }

    @Test
    fun `a NEW_TASK SEND intent yields its draft and a stripped intent yields nothing`() {
        val send = Intent(Intent.ACTION_SEND)
            .setType("text/plain")
            .putExtra(Intent.EXTRA_TEXT, "hello")
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        assertEquals("hello", shareDraftFromIntent(send))

        // What consumeLaunchIntent leaves behind for a later recreate/replay to re-read.
        val stripped = Intent(send).apply {
            data = null
            removeExtra(Intent.EXTRA_TEXT)
            removeExtra(Intent.EXTRA_SUBJECT)
        }
        assertNull(shareDraftFromIntent(stripped))
        assertNull(deepLinkUri(stripped))
    }
}
