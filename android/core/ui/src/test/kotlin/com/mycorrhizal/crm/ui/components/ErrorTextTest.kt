package com.mycorrhizal.crm.ui.components

import androidx.compose.material3.Text
import androidx.compose.ui.test.junit4.v2.createComposeRule
import com.mycorrhizal.crm.model.network.ErrorReason
import com.mycorrhizal.crm.model.network.ErrorReasonCode
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.NO_PENDING_TWO_FACTOR_BODY
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import java.lang.reflect.Modifier
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

    private companion object {
        const val SERVER = "Server fallback"
    }

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

    // setContent may run once per test, so several errors render in one pass.
    private fun renderAll(errors: List<ApiError>): List<String> {
        var texts = emptyList<String>()
        composeTestRule.setContent {
            MycorrhizalTheme {
                texts = errors.map { it.localizedMessage() }
                Text(texts.joinToString())
            }
        }
        composeTestRule.waitForIdle()
        return texts
    }

    @Test
    fun `the account lockout code renders retry_after as seconds`() {
        val error = ApiError.Client(
            429,
            SERVER,
            ErrorReason(ErrorReasonCode.AUTH_ACCOUNT_LOCKED, mapOf("retry_after" to 90.0))
        )
        assertEquals("Too many failed attempts. Try again in 90 s", render(error))
    }

    @Test
    fun `integration upstream and limit codes render their params and fall back without them`() {
        val rendered = renderAll(
            listOf(
                ApiError.Server(
                    503,
                    SERVER,
                    reason = ErrorReason(
                        ErrorReasonCode.IMMICH_UPSTREAM_ERROR,
                        mapOf("status" to "502 Bad Gateway")
                    )
                ),
                ApiError.Client(
                    409,
                    SERVER,
                    ErrorReason(ErrorReasonCode.FEED_LIMIT_REACHED, mapOf("max" to 50.0))
                ),
                ApiError.Client(
                    409,
                    SERVER,
                    ErrorReason(ErrorReasonCode.FEED_LIMIT_REACHED, emptyMap())
                )
            )
        )
        assertEquals(
            listOf(
                "Immich returned an error (502 Bad Gateway). " +
                    "The instance is reachable \u2014 this request itself failed.",
                "feed limit reached (maximum 50 active feeds)",
                SERVER
            ),
            rendered
        )
    }

    @Test
    fun `the account lockout code without retry_after falls back to the server message`() {
        val error = ApiError.Client(
            429,
            SERVER,
            ErrorReason(ErrorReasonCode.AUTH_ACCOUNT_LOCKED, emptyMap())
        )
        assertEquals(SERVER, render(error))
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
    fun `an invalid circle colour is localized`() {
        val error = ApiError.Client(
            400,
            "colour not in palette",
            ErrorReason(ErrorReasonCode.CIRCLE_COLOR_INVALID, emptyMap())
        )
        assertEquals("The colour must be one of the palette colours", render(error))
    }

    @Test
    fun `every mirrored code with full params renders its own template`() {
        val codes = ErrorReasonCode::class.java.declaredFields
            .filter { Modifier.isStatic(it.modifiers) && it.type == String::class.java }
            .map { it.apply { isAccessible = true }.get(null) as String }
        val params = mapOf(
            "field" to "f",
            "n" to 1.0,
            "limit" to 1.0,
            "required_version" to "1.0.0",
            "retry_after" to 90.0,
            "supported" to "en, de",
            "scope" to "api",
            "section" to "names",
            "reason" to "r",
            "total" to 1.0,
            "max" to 1.0,
            "status" to "s"
        )
        val rendered = mutableMapOf<String, String>()
        composeTestRule.setContent {
            MycorrhizalTheme {
                codes.forEach { code ->
                    rendered[code] = ApiError.Client(400, SERVER, ErrorReason(code, params))
                        .localizedMessage()
                }
            }
        }
        composeTestRule.waitForIdle()
        val fellBack = rendered.filterValues { it == SERVER }.keys
        assertEquals("codes that fell back to the server message", emptySet<String>(), fellBack)
    }

    @Test
    fun `a 4xx never shows the request reference`() {
        val error = ApiError.Client(400, "Server says no", requestId = "abc")
        assertEquals("Server says no", render(error))
    }

    @Test
    fun `client-worded errors render their translated strings`() {
        val rendered = renderAll(
            listOf(
                ApiError.Client(401, "x"),
                ApiError.Client(403, "x"),
                ApiError.Client(404, "x"),
                ApiError.Client(409, ""),
                ApiError.Client(418, ""),
                ApiError.Server(503, "hidden"),
                ApiError.PreconditionFailed("x"),
                ApiError.Parse("x"),
                ApiError.Client(401, NO_PENDING_TWO_FACTOR_BODY)
            )
        )
        assertEquals(
            listOf(
                "Session expired — please log in again",
                "You don't have permission",
                "Not found",
                "Conflict",
                "Request failed",
                "Server error (503)",
                "Changed elsewhere — load the latest version",
                "Unexpected server response",
                "No pending two-factor login found. Please sign in again."
            ),
            rendered
        )
    }

    @Test
    fun `an error that is not an API response uses the display message`() {
        assertEquals("Unexpected server response", render(ApiError.Parse("x")))
    }

    @Test
    fun `every param-free Meerkat code renders its translation, not the server text`() {
        val codes = listOf(
            ErrorReasonCode.MEERKAT_UPLOAD_MISSING to "No file uploaded",
            ErrorReasonCode.MEERKAT_SESSION_NOT_FOUND to "Import session expired or not found",
            ErrorReasonCode.MEERKAT_SESSION_NOT_OWNED to "Session does not belong to current user",
            ErrorReasonCode.MEERKAT_SESSION_EXPIRED to "Import session expired",
            ErrorReasonCode.MEERKAT_DB_SIZE to
                "The database file is empty or larger than the 100 MB limit",
            ErrorReasonCode.MEERKAT_UPLOAD_NOT_SQLITE to
                "Upload a Meerkat SQLite database (.db, .sqlite or .sqlite3)",
            ErrorReasonCode.MEERKAT_UPLOAD_UNREADABLE to "The upload could not be read",
            ErrorReasonCode.MEERKAT_FILE_NOT_MEERKAT to "That file is not a Meerkat database",
            ErrorReasonCode.MEERKAT_DB_UNREADABLE to
                "The database could not be read as a Meerkat database",
            ErrorReasonCode.MEERKAT_DB_TOO_MANY_CONTACTS to
                "This database has more contacts than the import supports",
            ErrorReasonCode.MEERKAT_DB_TOO_MANY_RECORDS to
                "This database has more records than the import supports",
            ErrorReasonCode.MEERKAT_IMPORT_IN_PROGRESS to "This import is already being prepared",
            ErrorReasonCode.MEERKAT_NOT_PREPARED to "The Meerkat database is not prepared yet",
            ErrorReasonCode.MEERKAT_SIGN_IN_INPUT_MISSING to
                "Enter a Meerkat API token, or a username and password",
            ErrorReasonCode.MEERKAT_VERSION_UNTESTED to
                "This Meerkat version is not tested with the importer. " +
                "Confirm to try anyway; nothing is written until you review the preview."
        )
        val rendered = renderAll(
            codes.map { (code, _) ->
                ApiError.Client(400, "Server says no", ErrorReason(code, emptyMap()))
            }
        )
        assertEquals(codes.map { it.second }, rendered)
    }

    @Test
    fun `the Meerkat upstream codes render the reason param and fall back without it`() {
        val codes = listOf(
            ErrorReasonCode.MEERKAT_API_ERROR,
            ErrorReasonCode.MEERKAT_UPSTREAM_ERROR
        )
        val rendered = renderAll(
            codes.flatMap { code ->
                listOf(
                    ApiError.Client(
                        502,
                        "Server says no",
                        ErrorReason(code, mapOf("reason" to "token rejected"))
                    ),
                    ApiError.Client(502, "Server says no", ErrorReason(code, emptyMap()))
                )
            }
        )
        assertEquals(
            listOf("token rejected", "Server says no", "token rejected", "Server says no"),
            rendered
        )
    }

    @Test
    fun `the Meerkat contact limit renders both counts and falls back when one is missing`() {
        val code = ErrorReasonCode.MEERKAT_REMOTE_TOO_MANY_CONTACTS
        val rendered = renderAll(
            listOf(
                ApiError.Client(
                    400,
                    "Server says no",
                    ErrorReason(code, mapOf("total" to 6000.0, "max" to 5000.0))
                ),
                ApiError.Client(
                    400,
                    "Server says no",
                    ErrorReason(code, mapOf("total" to 6000.0))
                )
            )
        )
        assertEquals(
            listOf(
                "Contacts on this Meerkat account: 6000. The import supports at most 5000",
                "Server says no"
            ),
            rendered
        )
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
