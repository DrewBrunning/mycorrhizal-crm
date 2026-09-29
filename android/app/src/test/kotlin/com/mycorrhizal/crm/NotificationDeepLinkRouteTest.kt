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
            val expected = if (v.isNull("android_route")) null else v.getString("android_route")
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

    @Test
    fun `search links sanitise the query and reject extra path or a foreign host`() {
        assertEquals("contacts", deepLinkRoute(Uri.parse("mycorrhizal://search")))
        assertEquals("contacts", deepLinkRoute(Uri.parse("mycorrhizal://search?q=%20%20")))
        assertEquals("contacts?search=ann", deepLinkRoute(Uri.parse("mycorrhizal://search?q=%20ann%20")))
        // Cf (zero-width space U+200B) and Cc (NUL) are stripped.
        assertEquals("contacts?search=ab", deepLinkRoute(Uri.parse("mycorrhizal://search?q=a%E2%80%8Bb")))
        assertEquals("contacts", deepLinkRoute(Uri.parse("mycorrhizal://search?q=%00")))
        assertEquals("contacts?search=a%20b%26c", deepLinkRoute(Uri.parse("mycorrhizal://search?q=a%20b%26c")))
        assertEquals("contacts?search=ann", deepLinkRoute(Uri.parse("mycorrhizal://search?x=1&q=ann")))
        assertEquals("contacts?search=" + "a".repeat(200), deepLinkRoute(Uri.parse("mycorrhizal://search?q=" + "a".repeat(250))))
        assertNull(deepLinkRoute(Uri.parse("mycorrhizal://search/extra?q=ann")))
    }

    @Test
    fun `deepLinkUri takes a non-oidc VIEW intent's data else the notification extra`() {
        val view = Intent(Intent.ACTION_VIEW, Uri.parse("mycorrhizal://contacts/5"))
        assertEquals("mycorrhizal://contacts/5", deepLinkUri(view).toString())

        // The OIDC callback is auth-only: never treated as a navigable link.
        val oidc = Intent(Intent.ACTION_VIEW, Uri.parse("mycorrhizal://oidc/callback?error=access_denied"))
        assertNull(deepLinkUri(oidc))

        // Data on a non-VIEW intent is ignored; the extra is used.
        val extra = Intent(Intent.ACTION_MAIN).apply {
            data = Uri.parse("mycorrhizal://contacts/9")
            putExtra(com.mycorrhizal.crm.feature.tracking.NotificationBuilder.EXTRA_DEEP_LINK, "mycorrhizal://home")
        }
        assertEquals("mycorrhizal://home", deepLinkUri(extra).toString())

        assertNull(deepLinkUri(Intent(Intent.ACTION_MAIN)))
        assertNull(deepLinkUri(Intent(Intent.ACTION_MAIN).putExtra(com.mycorrhizal.crm.feature.tracking.NotificationBuilder.EXTRA_DEEP_LINK, " ")))
        assertNull(deepLinkUri(null))
    }

    // ADR 0029: the manifest filter lists each allowed host explicitly — a scheme-only
    // filter would resolve mycorrhizal://settings. (The filter is advisory; deepLinkRoute
    // is the boundary.)
    @Test
    fun `the manifest resolves allowed deep-link hosts to MainActivity and nothing else`() {
        val pm = androidx.test.core.app.ApplicationProvider.getApplicationContext<Application>().packageManager
        fun resolves(uri: String) = pm.queryIntentActivities(
            Intent(Intent.ACTION_VIEW, Uri.parse(uri)).addCategory(Intent.CATEGORY_BROWSABLE),
            0,
        ).map { it.activityInfo.name }
        for (ok in listOf(
            "mycorrhizal://home", "mycorrhizal://contacts/1", "mycorrhizal://search?q=a",
            "mycorrhizal://circles/c", "mycorrhizal://tags/t", "mycorrhizal://households/h",
        )) {
            assertEquals(ok, listOf(MainActivity::class.java.name), resolves(ok))
        }
        assertTrue(resolves("mycorrhizal://settings").isEmpty())
        assertTrue(resolves("mycorrhizal://contacts").isNotEmpty())
    }

    // --- ADR 0029 §6 (issue #1271): share-to-CRM intake ---------------------------------

    private fun send(text: String?, subject: String? = null, type: String? = "text/plain", action: String = Intent.ACTION_SEND) =
        Intent(action).apply {
            this.type = type
            text?.let { putExtra(Intent.EXTRA_TEXT, it) }
            subject?.let { putExtra(Intent.EXTRA_SUBJECT, it) }
        }

    @Test
    fun `a text plain SEND intent yields the sanitised draft with the subject first`() {
        assertEquals("hello", shareDraftFromIntent(send("hello")))
        assertEquals("Subj\n\nhello", shareDraftFromIntent(send("hello", subject = "Subj")))
        assertEquals("ab\nc", shareDraftFromIntent(send("a\u0000b\nc\u200B")))
    }

    @Test
    fun `the draft is truncated to 10000 characters with an ellipsis`() {
        val draft = shareDraftFromIntent(send("y".repeat(10_050)))!!
        assertEquals("y".repeat(10_000) + "…", draft)
    }

    @Test
    fun `only ACTION_SEND text plain is accepted`() {
        assertNull(shareDraftFromIntent(send("hi", type = "text/x-vcard")))
        assertNull(shareDraftFromIntent(send("hi", type = "text/vcard")))
        assertNull(shareDraftFromIntent(send("hi", type = "image/png")))
        assertNull(shareDraftFromIntent(send("hi", type = null)))
        assertNull(shareDraftFromIntent(send("hi", action = Intent.ACTION_VIEW)))
        assertNull(shareDraftFromIntent(send("hi", action = Intent.ACTION_SEND_MULTIPLE)))
        assertNull(shareDraftFromIntent(null))
    }

    @Test
    fun `an empty or non-text share yields nothing`() {
        assertNull(shareDraftFromIntent(send(null)))
        assertNull(shareDraftFromIntent(send("  \n ")))
        val wrongType = Intent(Intent.ACTION_SEND).setType("text/plain").putExtra(Intent.EXTRA_TEXT, 42)
        assertNull(shareDraftFromIntent(wrongType))
    }

    @Test
    fun `a pending share expires strictly after the ttl`() {
        val share = PendingShare("t", receivedAtMillis = 1_000L)
        assertFalse(share.isExpired(1_000L + DEEP_LINK_TTL_MILLIS))
        assertTrue(share.isExpired(1_000L + DEEP_LINK_TTL_MILLIS + 1))
    }

    @Test
    fun `logging out clears the pending share but staying logged in keeps it`() = runTest {
        val share = PendingShare("t", 0L)
        val kept = MutableStateFlow<PendingShare?>(share)
        kept.clearWhenLoggedOut(flowOf(SessionState(isLoggedIn = true)))
        assertEquals(share, kept.value)

        val cleared = MutableStateFlow<PendingShare?>(share)
        cleared.clearWhenLoggedOut(flowOf(SessionState(isLoggedIn = true), SessionState(isLoggedIn = false)))
        assertNull(cleared.value)
    }

    @Test
    fun `share routes carry only an encoded key never the text`() {
        assertEquals("share/pick-contact?key=abc-123", sharePickerRoute("abc-123"))
        assertEquals("contacts/7/notes/new?prefill=abc-123", sharedNoteRoute(7, "abc-123"))
        assertEquals("share/pick-contact?key=a%26b", sharePickerRoute("a&b"))
    }

    @Test
    fun `the share intake view model stashes discards and clears drafts`() {
        val holder = com.mycorrhizal.crm.data.share.ShareDraftHolder()
        val vm = ShareIntakeViewModel(holder)
        val a = vm.stash("one")
        val b = vm.stash("two")
        vm.discard(a)
        assertNull(holder.take(a))
        vm.discardAll()
        assertNull(holder.take(b))
        // Stashed text is retrievable exactly once by the form.
        val c = vm.stash("three")
        assertEquals("three", holder.take(c))
        assertNull(holder.take(c))
    }

    @Test
    fun `the manifest offers ACTION_SEND text plain to MainActivity only`() {
        val pm = androidx.test.core.app.ApplicationProvider.getApplicationContext<Application>().packageManager
        fun resolves(type: String) = pm.queryIntentActivities(
            Intent(Intent.ACTION_SEND).setType(type),
            0,
        ).map { it.activityInfo.name }
        assertEquals(listOf(MainActivity::class.java.name), resolves("text/plain"))
        assertTrue(resolves("text/vcard").isEmpty())
        assertTrue(resolves("text/x-vcard").isEmpty())
        assertTrue(resolves("image/png").isEmpty())
    }
}
