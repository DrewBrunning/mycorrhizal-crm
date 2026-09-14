package com.mycorrhizal.crm

import android.app.Application
import android.net.Uri
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

// M5 §5 / issue #965: the OIDC native-return deep link parsing is pure so it
// can be tested without launching the Activity. Robolectric provides a real
// android.net.Uri; the plain Application avoids booting the @HiltAndroidApp one.
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
class OidcReturnParsingTest {

    @Test
    fun `a success callback captures the exchange code, state and profile prefs`() {
        val uri = Uri.parse(
            "mycorrhizal://oidc/callback?code=single-use-code&state=app-state&language=de&date_format=eu",
        )

        val parsed = parseOidcReturn(uri)

        assertEquals(
            OidcReturn.Success(state = "app-state", code = "single-use-code", language = "de", dateFormat = "eu"),
            parsed,
        )
    }

    @Test
    fun `language and date format are optional`() {
        val uri = Uri.parse("mycorrhizal://oidc/callback?code=abc&state=s")

        val parsed = parseOidcReturn(uri)

        assertEquals(OidcReturn.Success(state = "s", code = "abc", language = null, dateFormat = null), parsed)
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
    fun `a code on a different path of the oidc host is ignored`() {
        // MainActivity is exported, so the path is part of the contract too —
        // an explicit-component VIEW intent must not be able to inject a code.
        assertNull(parseOidcReturn(Uri.parse("mycorrhizal://oidc/other?code=abc&state=s")))
    }

    @Test
    fun `a code-less callback is ignored`() {
        assertNull(parseOidcReturn(Uri.parse("mycorrhizal://oidc/callback?state=s")))
    }

    @Test
    fun `a callback without a state nonce is ignored`() {
        // The state is what binds the callback to a flow this app started; a
        // code alone (e.g. another app/attacker initiating the flow) is not
        // accepted.
        assertNull(parseOidcReturn(Uri.parse("mycorrhizal://oidc/callback?code=abc")))
    }

    // Issue #965 regression: the pre-fix backend delivered the raw session JWT
    // in a `token` query parameter. That shape must stay unparseable — accepting
    // it would reintroduce the custom-scheme token theft the fix removed.
    @Test
    fun `a legacy token-bearing callback is ignored`() {
        assertNull(
            parseOidcReturn(
                Uri.parse("mycorrhizal://oidc/callback?token=eyJhbGciOiJIUzI1NiJ9.abc&language=de"),
            ),
        )
    }
}
