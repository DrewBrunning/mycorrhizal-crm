package com.mycorrhizal.crm.feature.contacts

import android.content.Context
import android.content.Intent
import android.net.Uri
import androidx.test.core.app.ApplicationProvider
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class FieldActionsTest {

    @Test
    fun `dial intent uses ACTION_DIAL with tel scheme`() {
        val intent = FieldActions.dialIntent("+1-555-0100")
        assertEquals(Intent.ACTION_DIAL, intent.action)
        assertEquals("tel", intent.data?.scheme)
        assertEquals("+1-555-0100", intent.data?.schemeSpecificPart)
    }

    @Test
    fun `dial intent trims whitespace`() {
        val intent = FieldActions.dialIntent("  +1-555-0100  ")
        assertEquals("+1-555-0100", intent.data?.schemeSpecificPart)
    }

    @Test
    fun `sms intent uses ACTION_SENDTO with smsto scheme`() {
        val intent = FieldActions.smsIntent("+1-555-0100")
        assertEquals(Intent.ACTION_SENDTO, intent.action)
        assertEquals("smsto", intent.data?.scheme)
        assertEquals("+1-555-0100", intent.data?.schemeSpecificPart)
    }

    @Test
    fun `email intent uses ACTION_SENDTO with mailto scheme`() {
        val intent = FieldActions.emailIntent("dana@example.com")
        assertEquals(Intent.ACTION_SENDTO, intent.action)
        assertEquals("mailto", intent.data?.scheme)
        assertEquals("dana@example.com", intent.data?.schemeSpecificPart)
    }

    @Test
    fun `map intent uses ACTION_VIEW with geo scheme and encoded query`() {
        val intent = FieldActions.mapIntent("123 Main St, Springfield")
        assertEquals(Intent.ACTION_VIEW, intent.action)
        assertEquals("geo", intent.data?.scheme)
        // The address must be URL-encoded in the q= query, not concatenated raw.
        val uriString = intent.data.toString()
        assertEquals("geo:0,0?q=123%20Main%20St%2C%20Springfield", uriString)
    }

    @Test
    fun `browser intent uses ACTION_VIEW with the url`() {
        val intent = FieldActions.browserIntent("https://example.com/profile")
        assertEquals(Intent.ACTION_VIEW, intent.action)
        assertEquals(Uri.parse("https://example.com/profile"), intent.data)
    }

    @Test
    fun `copy text places a clip on the clipboard`() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        FieldActions.copyText(context, "phone", "+1-555-0100")
        val clipboard = context.getSystemService(Context.CLIPBOARD_SERVICE) as android.content.ClipboardManager
        assertNotNull(clipboard.primaryClip)
        assertEquals("+1-555-0100", clipboard.primaryClip?.getItemAt(0)?.text?.toString())
    }

    @Test
    fun `copy text clip carries the label`() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        FieldActions.copyText(context, "email", "a@example.com")
        val clipboard = context.getSystemService(Context.CLIPBOARD_SERVICE) as android.content.ClipboardManager
        assertTrue(clipboard.primaryClip?.description?.label?.contains("email") == true)
    }
}
