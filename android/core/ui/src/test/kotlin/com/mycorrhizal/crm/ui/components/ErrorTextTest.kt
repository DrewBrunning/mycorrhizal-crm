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
 * Issue #2097 / ADR 0038: an error renders its translated reason in the UI
 * language, and falls back to the server's English message when the code, a
 * param or the mapping is missing.
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

    @Test
    fun `a known code renders its template with the params`() {
        val error = ApiError.Client(
            400,
            "must be at most 10 characters",
            ErrorReason(
                ErrorReasonCode.CONTACT_BULK_VALUE_TOO_LONG,
                mapOf("field" to "organization", "limit" to 10.0)
            )
        )
        assertEquals("organization length limit is 10", render(error))
    }

    @Test
    fun `an unknown code falls back to the server message`() {
        val error = ApiError.Client(
            400,
            "Server says no",
            ErrorReason("not_a_real_code", emptyMap())
        )
        assertEquals("Server says no", render(error))
    }

    @Test
    fun `a missing param falls back to the server message`() {
        val error = ApiError.Client(
            400,
            "Server says no",
            ErrorReason(
                ErrorReasonCode.CONTACT_BULK_VALUE_TOO_LONG,
                mapOf("field" to "organization")
            )
        )
        assertEquals("Server says no", render(error))
    }

    @Test
    fun `a single-param code renders its template`() {
        val error = ApiError.Client(
            400,
            "Server says no",
            ErrorReason(
                ErrorReasonCode.CONTACT_BULK_FIELD_NOT_EDITABLE,
                mapOf("field" to "email")
            )
        )
        assertEquals("email is not bulk-editable", render(error))
    }

    @Test
    fun `a single-param code with a missing param falls back`() {
        val error = ApiError.Client(
            400,
            "Server says no",
            ErrorReason(ErrorReasonCode.CONTACT_BULK_FIELD_NOT_EDITABLE, emptyMap())
        )
        assertEquals("Server says no", render(error))
    }

    @Test
    fun `an error without a reason uses the display message`() {
        assertEquals("Server says no", render(ApiError.Client(400, "Server says no")))
    }

    @Test
    fun `a precondition failure with a reason is localized`() {
        val error = ApiError.PreconditionFailed(
            "Server says no",
            ErrorReason(ErrorReasonCode.RELATIONSHIP_SELF_EDGE, emptyMap())
        )
        assertEquals("The source and target must be different contacts", render(error))
    }

    @Test
    fun `a trash uid-in-use reason is localized`() {
        val error = ApiError.Client(
            409,
            "server text",
            ErrorReason(ErrorReasonCode.TRASH_UID_IN_USE, emptyMap())
        )
        assertEquals(
            "A live contact already uses this contact's UID; merge or delete it first",
            render(error)
        )
    }

    @Test
    fun `an unsupported trash snapshot reason is localized`() {
        val error = ApiError.Client(
            409,
            "server text",
            ErrorReason(ErrorReasonCode.TRASH_SNAPSHOT_UNSUPPORTED, emptyMap())
        )
        assertEquals(
            "This contact's deletion snapshot is not supported by this version; " +
                "it can still be deleted permanently",
            render(error)
        )
    }

    @Test
    fun `a 4xx never shows the request reference`() {
        val error = ApiError.Client(400, "Server says no", requestId = "abc")
        assertEquals("Server says no", render(error))
    }

    @Test
    fun `an error that is not an API response uses the display message`() {
        assertEquals("Unexpected server response", render(ApiError.Parse("x")))
    }

    @Test
    fun `a 5xx appends the request reference`() {
        val error = ApiError.Server(503, "Server error", requestId = "abc")
        assertEquals("Server error (503) (ref: abc)", render(error))
    }
}

/** The same translation under the German qualifier (the app's de locale). */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], qualifiers = "de")
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ErrorTextGermanTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun `a known code renders in German`() {
        var text = ""
        composeTestRule.setContent {
            MycorrhizalTheme {
                text = ApiError.Client(
                    400,
                    "The source and target must be different contacts",
                    ErrorReason(ErrorReasonCode.RELATIONSHIP_SELF_EDGE, emptyMap())
                ).localizedMessage()
                Text(text)
            }
        }
        composeTestRule.waitForIdle()
        assertEquals("Quelle und Ziel müssen verschiedene Kontakte sein", text)
    }
}
