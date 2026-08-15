package com.mycorrhizal.crm

import android.app.Application
import android.net.Uri
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

// M5 §5: the OIDC native-return deep link parsing is pure so it can be tested
// without launching the Activity. Robolectric provides a real android.net.Uri;
// the plain Application avoids booting the @HiltAndroidApp one.
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
class OidcReturnParsingTest {

    @Test
    fun `a success callback captures the token and profile prefs`() {
        val uri = Uri.parse(
            "mycorrhizal://oidc/callback?token=eyJhbGciOiJIUzI1NiJ9.abc&language=de&date_format=eu",
        )

        val parsed = parseOidcReturn(uri)

        assertEquals(OidcReturn.Success(token = "eyJhbGciOiJIUzI1NiJ9.abc", language = "de", dateFormat = "eu"), parsed)
    }

    @Test
    fun `language and date format are optional`() {
        val uri = Uri.parse("mycorrhizal://oidc/callback?token=abc")

        val parsed = parseOidcReturn(uri)

        assertEquals(OidcReturn.Success(token = "abc", language = null, dateFormat = null), parsed)
    }

    @Test
    fun `an error callback maps to failure`() {
        val uri = Uri.parse("mycorrhizal://oidc/callback?error=access_denied")

        assertEquals(OidcReturn.Failure, parseOidcReturn(uri))
    }

    @Test
    fun `unrelated uris are ignored`() {
        assertNull(parseOidcReturn(null))
        assertNull(parseOidcReturn(Uri.parse("https://example.com/")))
        assertNull(parseOidcReturn(Uri.parse("mycorrhizal://other/route")))
    }

    @Test
    fun `a token on a different path of the oidc host is ignored`() {
        // MainActivity is exported, so the path is part of the contract too —
        // an explicit-component VIEW intent must not be able to inject a token.
        assertNull(parseOidcReturn(Uri.parse("mycorrhizal://oidc/other?token=abc")))
    }

    @Test
    fun `a token-less success is ignored`() {
        assertNull(parseOidcReturn(Uri.parse("mycorrhizal://oidc/callback")))
    }
}
