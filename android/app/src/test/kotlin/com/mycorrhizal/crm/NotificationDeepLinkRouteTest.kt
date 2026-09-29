package com.mycorrhizal.crm

import android.app.Application
import android.net.Uri
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.json.JSONObject
import android.content.Intent
import com.mycorrhizal.crm.domain.repository.SessionState
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.test.runTest
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

    @Test
    fun `lenient integer ids and path tricks are rejected`() {
        assertNull(deepLinkRoute(Uri.parse("mycorrhizal://contacts/+42")))
        assertNull(deepLinkRoute(Uri.parse("mycorrhizal://contacts/042")))
        assertNull(deepLinkRoute(Uri.parse("mycorrhizal://contacts/2147483648")))
        assertNull(deepLinkRoute(Uri.parse("mycorrhizal://contacts/99999999999")))
        assertNull(deepLinkRoute(Uri.parse("mycorrhizal://contacts//42")))
        assertNull(deepLinkRoute(Uri.parse("mycorrhizal://circles/%2e%2e")))
        assertNull(deepLinkRoute(Uri.parse("mycorrhizal://circles/.")))
        assertNull(deepLinkRoute(Uri.parse("mycorrhizal://circles/a%2Fb")))
        assertNull(deepLinkRoute(Uri.parse("mycorrhizal://circles/a b")))
        assertNull(deepLinkRoute(Uri.parse("mycorrhizal://circles/" + "a".repeat(129))))
        assertEquals("circles/" + "a".repeat(128), deepLinkRoute(Uri.parse("mycorrhizal://circles/" + "a".repeat(128))))
        assertNull(deepLinkRoute(Uri.parse("mycorrhizal://home/extra")))
    }

    @Test
    fun `every shared vector resolves to its android route`() {
        val text = javaClass.classLoader!!.getResourceAsStream("vectors.json")!!
            .bufferedReader().use { it.readText() }
        val vectors = JSONObject(text).getJSONArray("vectors")
        assertTrue(vectors.length() > 0)
        for (i in 0 until vectors.length()) {
            val v = vectors.getJSONObject(i)
            val uri = v.getString("uri")
            var expected = if (v.isNull("android_route")) null else v.getString("android_route")
            // TODO(#1269): the search route does not exist yet; until it does the
            // parser must reject the search host (keyed on host, not blanket).
            if (Uri.parse(uri).host == "search") expected = null
            assertEquals("vector $uri", expected, deepLinkRoute(Uri.parse(uri)))
        }
    }

    @Test
    fun `shouldHandleLaunchIntent only accepts a fresh non-history launch`() {
        val history = Intent.FLAG_ACTIVITY_LAUNCHED_FROM_HISTORY
        assertTrue(shouldHandleLaunchIntent(savedStateIsNull = true, flags = 0))
        assertFalse(shouldHandleLaunchIntent(savedStateIsNull = false, flags = 0))
        assertFalse(shouldHandleLaunchIntent(savedStateIsNull = true, flags = history))
        assertFalse(shouldHandleLaunchIntent(savedStateIsNull = false, flags = history))
        assertTrue(shouldHandleLaunchIntent(savedStateIsNull = true, flags = Intent.FLAG_ACTIVITY_NEW_TASK))
    }

    @Test
    fun `a pending link expires strictly after the ttl`() {
        val link = PendingDeepLink(Uri.parse("mycorrhizal://home"), receivedAtMillis = 1_000L)
        assertFalse(link.isExpired(1_000L + DEEP_LINK_TTL_MILLIS))
        assertTrue(link.isExpired(1_000L + DEEP_LINK_TTL_MILLIS + 1))
        assertEquals(10 * 60 * 1000L, DEEP_LINK_TTL_MILLIS)
    }

    @Test
    fun `logging out clears the pending link but staying logged in keeps it`() = runTest {
        val link = PendingDeepLink(Uri.parse("mycorrhizal://home"), 0L)
        val kept = MutableStateFlow<PendingDeepLink?>(link)
        kept.clearWhenLoggedOut(flowOf(SessionState(isLoggedIn = true)))
        assertEquals(link, kept.value)

        val cleared = MutableStateFlow<PendingDeepLink?>(link)
        cleared.clearWhenLoggedOut(flowOf(SessionState(isLoggedIn = true), SessionState(isLoggedIn = false)))
        assertNull(cleared.value)
    }
}
