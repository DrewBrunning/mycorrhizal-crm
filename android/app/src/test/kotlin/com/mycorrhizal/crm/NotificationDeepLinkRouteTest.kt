package com.mycorrhizal.crm

import android.app.Application
import android.net.Uri
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

// Issue #679: deep-link → NavHost-route parsing is pure so it can be tested
// without launching the Activity. Robolectric provides a real android.net.Uri;
// the plain Application avoids booting the @HiltAndroidApp. Every route here is
// a route that actually exists in the NavHost; malformed or foreign links must
// degrade to null (ADR-0002), never drive navigation.
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
class NotificationDeepLinkRouteTest {

    @Test
    fun `a contacts deep link maps to the contact route`() {
        assertEquals("contacts/7", deepLinkRoute(Uri.parse("mycorrhizal://contacts/7")))
    }

    @Test
    fun `a nested activities deep link maps to the activities route`() {
        assertEquals(
            "contacts/7/activities",
            deepLinkRoute(Uri.parse("mycorrhizal://contacts/7/activities")),
        )
    }

    @Test
    fun `a dashboard deep link maps to the home route`() {
        assertEquals("home", deepLinkRoute(Uri.parse("mycorrhizal://home")))
    }

    @Test
    fun `string-id deep links map to their circle tag and household routes`() {
        assertEquals("circles/c-9", deepLinkRoute(Uri.parse("mycorrhizal://circles/c-9")))
        assertEquals("tags/co-workers", deepLinkRoute(Uri.parse("mycorrhizal://tags/co-workers")))
        assertEquals("households/h-3", deepLinkRoute(Uri.parse("mycorrhizal://households/h-3")))
    }

    @Test
    fun `unrelated uris are ignored`() {
        assertNull(deepLinkRoute(null))
        assertNull(deepLinkRoute(Uri.parse("https://example.com/contacts/7")))
        assertNull(deepLinkRoute(Uri.parse("mycorrhizal://other/7")))
        assertNull(deepLinkRoute(Uri.parse("mycorrhizal://oidc/callback?token=abc")))
    }

    @Test
    fun `a non-numeric or non-positive contact id is ignored`() {
        assertNull(deepLinkRoute(Uri.parse("mycorrhizal://contacts/abc")))
        assertNull(deepLinkRoute(Uri.parse("mycorrhizal://contacts/0")))
        assertNull(deepLinkRoute(Uri.parse("mycorrhizal://contacts/-3")))
    }

    @Test
    fun `malformed contact sub-routes are ignored`() {
        assertNull(deepLinkRoute(Uri.parse("mycorrhizal://contacts")))
        assertNull(deepLinkRoute(Uri.parse("mycorrhizal://contacts/")))
        assertNull(deepLinkRoute(Uri.parse("mycorrhizal://contacts/7/notes")))
        assertNull(deepLinkRoute(Uri.parse("mycorrhizal://contacts/7/activities/9")))
        assertNull(deepLinkRoute(Uri.parse("mycorrhizal://contacts/7/activities/")))
    }

    @Test
    fun `string-id routes reject blank or multi-segment ids`() {
        assertNull(deepLinkRoute(Uri.parse("mycorrhizal://circles")))
        assertNull(deepLinkRoute(Uri.parse("mycorrhizal://tags/")))
        assertNull(deepLinkRoute(Uri.parse("mycorrhizal://households/a/b")))
    }
}
