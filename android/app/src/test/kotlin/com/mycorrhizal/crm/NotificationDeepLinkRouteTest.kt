package com.mycorrhizal.crm

import android.app.Application
import android.net.Uri
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

// M5 §6.6 (issue #152): notification deep-link → NavHost-route parsing is pure
// so it can be tested without launching the Activity. Robolectric provides a
// real android.net.Uri; the plain Application avoids booting the @HiltAndroidApp.
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
class NotificationDeepLinkRouteTest {

    @Test
    fun `a contacts deep link maps to the contact route`() {
        assertEquals("contacts/7", deepLinkRoute(Uri.parse("mycorrhizal://contacts/7")))
    }

    @Test
    fun `unrelated uris are ignored`() {
        assertNull(deepLinkRoute(null))
        assertNull(deepLinkRoute(Uri.parse("https://example.com/contacts/7")))
        assertNull(deepLinkRoute(Uri.parse("mycorrhizal://other/7")))
    }

    @Test
    fun `a non-numeric or non-positive contact id is ignored`() {
        assertNull(deepLinkRoute(Uri.parse("mycorrhizal://contacts/abc")))
        assertNull(deepLinkRoute(Uri.parse("mycorrhizal://contacts/0")))
        assertNull(deepLinkRoute(Uri.parse("mycorrhizal://contacts/-3")))
    }
}
