package com.mycorrhizal.crm.ui.components

import androidx.compose.material3.Text
import androidx.compose.ui.test.junit4.v2.createComposeRule
import com.mycorrhizal.crm.model.network.ErrorReason
import com.mycorrhizal.crm.model.network.ErrorReasonCode
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issue #2097 / ADR 0038: a server error's reason_code + reason_params render
 * in the UI language, with the server's English as the fallback.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ErrorTextTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun render(error: ApiError): String {
        var text = ""
        composeTestRule.setContent {
            MycorrhizalTheme {
                text = error.localizedMessage()
                Text(text)
            }
        }
        composeTestRule.waitForIdle()
        return text
    }

    private fun reason(params: Map<String, Any?> = mapOf("n" to 3L)) =
        ErrorReason(ErrorReasonCode.COMMON_TEST_ONLY, params)

    @Test
    fun `known code and params render the localized string`() {
        assertEquals("Test 3", render(ApiError.Client(400, "Server English", reason())))
    }

    @Test
    @Config(qualifiers = "de")
    fun `known code renders German under the de qualifier`() {
        assertEquals("Prüfung 3", render(ApiError.Client(400, "Server English", reason())))
    }

    @Test
    fun `unknown code falls back to the display message`() {
        val unknown = ErrorReason("nope_nope", mapOf("n" to 3L))
        assertEquals("Server English", render(ApiError.Client(400, "Server English", unknown)))
    }

    @Test
    fun `missing param falls back to the display message`() {
        assertEquals(
            "Server English",
            render(ApiError.Client(400, "Server English", reason(emptyMap())))
        )
    }

    @Test
    fun `no reason uses the display message`() {
        assertEquals("Server English", render(ApiError.Client(400, "Server English")))
    }

    @Test
    fun `precondition failed with a reason is localized`() {
        assertEquals("Test 3", render(ApiError.PreconditionFailed("x", reason())))
    }

    @Test
    fun `5xx appends the translated request ref`() {
        assertEquals(
            "Server error (503) (ref: abc)",
            render(ApiError.Server(503, "boom", requestId = "abc"))
        )
    }

    @Test
    @Config(qualifiers = "de")
    fun `5xx ref is translated`() {
        assertEquals(
            "Server error (503) (Ref.: abc)",
            render(ApiError.Server(503, "boom", requestId = "abc"))
        )
    }

    @Test
    fun `4xx never shows the ref`() {
        val error = ApiError.Client(400, "Server English", requestId = "abc")
        assertEquals("Server English", render(error))
    }

    @Test
    fun `other errors use the display message`() {
        assertEquals("Unexpected server response", render(ApiError.Parse("x")))
    }
}
