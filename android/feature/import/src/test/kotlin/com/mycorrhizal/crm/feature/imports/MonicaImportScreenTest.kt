package com.mycorrhizal.crm.feature.imports

import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.assertWidthIsAtLeast
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.model.network.DuplicateMatch
import com.mycorrhizal.crm.model.network.ErrorReason
import com.mycorrhizal.crm.model.network.ErrorReasonCode
import com.mycorrhizal.crm.model.network.ImportRowIssue
import com.mycorrhizal.crm.model.network.ImportRowPreview
import com.mycorrhizal.crm.model.network.MonicaConnectResponse
import com.mycorrhizal.crm.model.network.MonicaEntityCounts
import com.mycorrhizal.crm.model.network.SourceImportIssue
import com.mycorrhizal.crm.model.network.SourceImportPreviewResponse
import com.mycorrhizal.crm.model.network.SourceImportResult
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/** Issue #2525: each step of the Monica flow renders real content and is wired to its callbacks. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class MonicaImportScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(
        uiState: MonicaImportUiState,
        onConnect: () -> Unit = {},
        onStart: () -> Unit = {},
        onIncludeRelationshipsChange: (Boolean) -> Unit = {},
        onIncludeExtrasChange: (Boolean) -> Unit = {},
        onRowActionChange: (Int, String) -> Unit = { _, _ -> },
        onConfirm: () -> Unit = {},
        onDone: () -> Unit = {}
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                MonicaImportScreenContent(
                    uiState = uiState,
                    onConnect = onConnect,
                    onStart = onStart,
                    onIncludeRelationshipsChange = onIncludeRelationshipsChange,
                    onIncludeExtrasChange = onIncludeExtrasChange,
                    onRowActionChange = onRowActionChange,
                    onConfirm = onConfirm,
                    onDone = onDone
                )
            }
        }
    }

    private fun connected() = MonicaConnectResponse(
        sessionId = "s1",
        totals = MonicaEntityCounts(contacts = 12, activities = 4, notes = 1),
        estimatedFetchSeconds = 95
    )

    @Test
    fun `the connect form renders and Connect is disabled until address and token are filled`() {
        setContent(MonicaImportUiState())
        composeTestRule.onNodeWithTag("monica-import-url").assertIsDisplayed()
        composeTestRule.onNodeWithTag("monica-import-token").assertIsDisplayed()
        composeTestRule.onNodeWithTag("monica-import-connect").assertIsNotEnabled()
    }

    @Test
    fun `Connect is enabled and fires once the form is complete`() {
        var connects = 0
        setContent(
            MonicaImportUiState(baseUrl = "https://m", apiToken = "t"),
            onConnect = { connects++ }
        )
        composeTestRule.onNodeWithTag("monica-import-connect")
            .performScrollTo().assertIsEnabled().performClick()
        assertEquals(1, connects)
    }

    @Test
    fun `the API token field is a password field`() {
        setContent(MonicaImportUiState(baseUrl = "https://m", apiToken = "supersecret"))
        composeTestRule.onNodeWithTag("monica-import-token")
            .assert(SemanticsMatcher.keyIsDefined(SemanticsProperties.Password))
        composeTestRule.onNodeWithTag("monica-import-url")
            .assert(SemanticsMatcher.keyNotDefined(SemanticsProperties.Password))
    }

    @Test
    fun `the fetch option switches report their toggles`() {
        val relationships = mutableListOf<Boolean>()
        val extras = mutableListOf<Boolean>()
        setContent(
            MonicaImportUiState(includeRelationships = true, includeExtras = false),
            onIncludeRelationshipsChange = { relationships += it },
            onIncludeExtrasChange = { extras += it }
        )
        composeTestRule.onNodeWithTag(
            "monica-import-relationships"
        ).performScrollTo().performClick()
        composeTestRule.onNodeWithTag("monica-import-extras").performScrollTo().performClick()
        assertEquals(listOf(false), relationships)
        assertEquals(listOf(true), extras)
    }

    @Test
    fun `a connection shows the counts and estimate, locks the form, and Start fires`() {
        var starts = 0
        setContent(
            MonicaImportUiState(baseUrl = "https://m", connected = connected()),
            onStart = { starts++ }
        )
        val found = composeTestRule.onNodeWithTag("monica-import-found")
        found.assertTextContains("12 contacts", substring = true)
        found.assertTextContains("4 activities", substring = true)
        found.assertTextContains("1 note", substring = true)
        composeTestRule.onNodeWithTag("monica-import-estimate")
            .assertTextContains("2 min", substring = true)
        composeTestRule.onNodeWithTag("monica-import-url").assertIsNotEnabled()
        composeTestRule.onNodeWithTag("monica-import-connect").assertDoesNotExist()
        composeTestRule.onNodeWithTag("monica-import-start")
            .performScrollTo().assertIsEnabled().performClick()
        assertEquals(1, starts)
    }

    @Test
    fun `a very short estimate still reads as at least one minute`() {
        setContent(
            MonicaImportUiState(
                connected = MonicaConnectResponse(
                    "s1",
                    MonicaEntityCounts(),
                    estimatedFetchSeconds = 5
                )
            )
        )
        composeTestRule.onNodeWithTag("monica-import-estimate")
            .assertTextContains("1 min", substring = true)
    }

    @Test
    fun `a Local profile shows the unavailable note instead of the form`() {
        setContent(MonicaImportUiState(unavailable = true))
        composeTestRule.onNodeWithTag("monica-import-unavailable").assertIsDisplayed()
        composeTestRule.onNodeWithTag("monica-import-url").assertDoesNotExist()
    }

    @Test
    fun `the working step shows progress text`() {
        setContent(MonicaImportUiState(step = MonicaImportStep.WORKING, isImporting = true))
        composeTestRule.onNodeWithTag("monica-import-working").assertIsDisplayed()
    }

    private fun reviewState(loss: List<SourceImportIssue>) = MonicaImportUiState(
        step = MonicaImportStep.REVIEW,
        preview = SourceImportPreviewResponse(
            sessionId = "s1",
            rows = listOf(
                ImportRowPreview(
                    rowIndex = 7,
                    parsedContact = mapOf("firstname" to "Ada", "lastname" to "Lovelace"),
                    duplicateMatch = DuplicateMatch(
                        existingContactId = 3,
                        existingFirstname = "Ada",
                        matchReason = "name"
                    ),
                    suggestedAction = "update"
                )
            ),
            totalRows = 1,
            validRows = 1,
            lossReport = loss
        ),
        rowActions = mapOf(7 to "update")
    )

    @Test
    fun `the review step lists rows and shows the loss summary only when there is loss`() {
        setContent(reviewState(emptyList()))
        composeTestRule.onNodeWithTag("import-review-list").assertIsDisplayed()
        composeTestRule.onNodeWithTag("monica-import-loss").assertDoesNotExist()
    }

    @Test
    fun `the review step shows the loss count`() {
        setContent(reviewState(listOf(SourceImportIssue("Ada", "photo", "dropped", "x"))))
        composeTestRule.onNodeWithTag("monica-import-loss")
            .assertIsDisplayed().assertTextContains("1 item", substring = true)
    }

    @Test
    fun `choosing a per-row action and confirming reach their callbacks`() {
        val picked = mutableListOf<Pair<Int, String>>()
        var confirms = 0
        setContent(
            reviewState(emptyList()),
            onRowActionChange = { index, action -> picked += index to action },
            onConfirm = { confirms++ }
        )
        composeTestRule.onNodeWithText("Discard new").performClick()
        composeTestRule.onNodeWithText("Keep both").performClick()
        composeTestRule.onNodeWithText("Merge").performClick()
        assertEquals(listOf(7 to "skip", 7 to "add", 7 to "update"), picked)
        composeTestRule.onNodeWithText("Apply", substring = true).performClick()
        assertEquals(1, confirms)
    }

    @Test
    fun `the result step shows the outcome and Done fires`() {
        var done = 0
        setContent(
            MonicaImportUiState(
                step = MonicaImportStep.RESULT,
                result = SourceImportResult(created = 3, updated = 1, skipped = 2)
            ),
            onDone = { done++ }
        )
        composeTestRule.onNodeWithTag("monica-import-result")
            .assertTextContains("3 created, 1 updated, 2 skipped", substring = true)
        composeTestRule.onNodeWithTag("monica-import-done").performClick()
        assertEquals(1, done)
    }

    @Test
    fun `the result step lists skipped rows with their reasons`() {
        setContent(
            MonicaImportUiState(
                step = MonicaImportStep.RESULT,
                result = SourceImportResult(
                    created = 3,
                    skipped = 1,
                    issues = listOf(
                        ImportRowIssue(
                            row = 4,
                            name = "Ada",
                            status = "skipped",
                            reason = "already exists"
                        )
                    )
                )
            )
        )

        composeTestRule.onNodeWithText("Row 4 · Ada").assertIsDisplayed()
        composeTestRule.onNodeWithText("Skipped: already exists").assertIsDisplayed()
    }

    @Test
    fun `a typed error is shown through its localized message`() {
        // Issue #2603: the old case used a reason-less error, which passes even when the screen
        // renders the raw server text. A reason code with a param proves the translation is used.
        setContent(
            MonicaImportUiState(
                error = ApiError.Client(
                    400,
                    "Monica rejected the API token",
                    reason = ErrorReason(
                        ErrorReasonCode.CONTACT_BULK_FIELD_NOT_EDITABLE,
                        mapOf("field" to "email")
                    )
                )
            )
        )
        composeTestRule.onNodeWithText("email is not bulk-editable").assertIsDisplayed()
        composeTestRule.onNodeWithText("Monica rejected the API token").assertDoesNotExist()
    }

    @Test
    fun `a job failure text is shown`() {
        setContent(MonicaImportUiState(errorText = "The Monica instance could not be reached"))
        composeTestRule.onNodeWithText(
            "The Monica instance could not be reached"
        ).assertIsDisplayed()
    }

    @Test
    fun `an error with a reason code is worded from the reason, never the server text`() {
        setContent(
            MonicaImportUiState(
                error = ApiError.Client(
                    409,
                    "RAW SERVER TEXT",
                    reason = ErrorReason(ErrorReasonCode.ENTITY_NOT_FOUND, emptyMap())
                )
            )
        )
        composeTestRule.onNodeWithText("Organisation, place or pet not found").assertIsDisplayed()
        composeTestRule.onNodeWithText("RAW SERVER TEXT").assertDoesNotExist()
    }

    // Issue #2620: the address field carries a sticky, localized unreachable message.
    @Test
    fun `an unreachable Monica is explained above Connect with the host and ref`() {
        setContent(
            MonicaImportUiState(
                baseUrl = "https://monica.example",
                apiToken = "tok",
                unreachable = MonicaUnreachable("monica.example", "req-9")
            )
        )
        composeTestRule.onNodeWithTag("monica-import-unreachable").performScrollTo()
            .assertTextContains("Couldn't reach Monica at monica.example", substring = true)
        composeTestRule.onNodeWithText("(ref: req-9)", substring = true)
            .performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("monica service error", substring = true)
            .assertDoesNotExist()
    }

    // #2951: same layout as Meerkat: a full-width Connect with the error directly above it
    // (below the form fields), not tucked under the address field.
    @Test
    fun `connect is full width with the unreachable error directly above it`() {
        setContent(
            MonicaImportUiState(
                baseUrl = "https://monica.example",
                apiToken = "t",
                unreachable = MonicaUnreachable("monica.example", "req-9")
            )
        )

        val connect = composeTestRule.onNodeWithTag("monica-import-connect")
            .performScrollTo().assertWidthIsAtLeast(280.dp).fetchSemanticsNode().boundsInRoot
        val error = composeTestRule.onNodeWithTag("monica-import-unreachable")
            .fetchSemanticsNode().boundsInRoot
        val token = composeTestRule.onNodeWithTag("monica-import-token")
            .fetchSemanticsNode().boundsInRoot
        assertTrue("error ends at or above Connect", error.bottom <= connect.top + 1f)
        assertTrue(
            "error sits below the token field, not under the address",
            error.top >= token.bottom
        )
    }

    @Test
    fun `the unreachable notice omits the ref line when the server sent none`() {
        setContent(
            MonicaImportUiState(
                baseUrl = "https://monica.example",
                unreachable = MonicaUnreachable("monica.example", null)
            )
        )
        composeTestRule.onNodeWithTag("monica-import-unreachable").performScrollTo()
            .assertIsDisplayed()
        composeTestRule.onNodeWithText("(ref:", substring = true).assertDoesNotExist()
    }

    @Test
    fun `no unreachable notice is shown by default`() {
        setContent(MonicaImportUiState())
        composeTestRule.onNodeWithTag("monica-import-unreachable").assertDoesNotExist()
    }

    @Test
    fun `an unreachable reason code never renders the raw monica service error prefix`() {
        setContent(
            MonicaImportUiState(
                error = ApiError.Server(
                    502,
                    "monica service error: The Monica instance could not be reached",
                    reason = ErrorReason(ErrorReasonCode.MONICA_UNREACHABLE, emptyMap())
                )
            )
        )
        composeTestRule.onNodeWithText(
            "The Monica instance could not be reached"
        ).assertIsDisplayed()
        composeTestRule.onNodeWithText("monica service error", substring = true)
            .assertDoesNotExist()
    }
}
