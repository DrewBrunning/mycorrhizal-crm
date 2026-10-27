package com.mycorrhizal.crm.feature.imports

import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import com.mycorrhizal.crm.model.network.ImportRowPreview
import com.mycorrhizal.crm.model.network.MeerkatConnectResponse
import com.mycorrhizal.crm.model.network.MeerkatEntityCounts
import com.mycorrhizal.crm.model.network.MeerkatVersionInfo
import com.mycorrhizal.crm.model.network.SourceImportIssue
import com.mycorrhizal.crm.model.network.SourceImportPreviewResponse
import com.mycorrhizal.crm.model.network.SourceImportResult
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/** Issue #2013: each step of the Meerkat flow renders real content (reachability proof). */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class MeerkatImportScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(
        uiState: MeerkatImportUiState,
        onConnect: () -> Unit = {},
        onStart: () -> Unit = {},
        onAcceptUntestedChange: (Boolean) -> Unit = {},
        onAuthModeChange: (MeerkatAuthMode) -> Unit = {},
        onDone: () -> Unit = {}
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                MeerkatImportScreenContent(
                    uiState = uiState,
                    onConnect = onConnect,
                    onStart = onStart,
                    onAcceptUntestedChange = onAcceptUntestedChange,
                    onAuthModeChange = onAuthModeChange,
                    onDone = onDone
                )
            }
        }
    }

    private fun connected(
        supported: Boolean = true,
        insecure: Boolean = false,
        reason: String = "newer than tested",
        reasonCode: String = ""
    ) = MeerkatConnectResponse(
        sessionId = "s1",
        version = MeerkatVersionInfo("v1.5-v1.7", supported, reason, reasonCode),
        totals = MeerkatEntityCounts(contacts = 12),
        insecureTransport = insecure
    )

    @Test
    fun `the connect form renders and Connect is disabled until address and token are filled`() {
        setContent(MeerkatImportUiState())
        composeTestRule.onNodeWithTag("meerkat-import-url").assertIsDisplayed()
        composeTestRule.onNodeWithTag("meerkat-import-token").assertIsDisplayed()
        composeTestRule.onNodeWithTag("meerkat-import-connect").assertIsNotEnabled()
    }

    @Test
    fun `Connect is enabled and fires once the form is complete`() {
        var connects = 0
        setContent(
            MeerkatImportUiState(baseUrl = "https://m", apiToken = "t"),
            onConnect = { connects++ }
        )
        composeTestRule.onNodeWithTag("meerkat-import-connect").assertIsEnabled().performClick()
        assertEquals(1, connects)
    }

    @Test
    fun `password mode swaps the token field for username and password`() {
        var mode: MeerkatAuthMode? = null
        setContent(
            MeerkatImportUiState(authMode = MeerkatAuthMode.PASSWORD),
            onAuthModeChange = { mode = it }
        )
        composeTestRule.onNodeWithTag("meerkat-import-username").assertIsDisplayed()
        composeTestRule.onNodeWithTag("meerkat-import-password").assertIsDisplayed()
        composeTestRule.onNodeWithTag("meerkat-import-token").assertDoesNotExist()
        composeTestRule.onNodeWithTag("meerkat-import-mode-token").performClick()
        assertEquals(MeerkatAuthMode.TOKEN, mode)
    }

    @Test
    fun `a supported connection shows the count and Start is enabled`() {
        var starts = 0
        setContent(
            MeerkatImportUiState(baseUrl = "https://m", connected = connected()),
            onStart = { starts++ }
        )
        composeTestRule.onNodeWithTag("meerkat-import-found").assertIsDisplayed()
        composeTestRule.onNodeWithTag("meerkat-import-untested").assertDoesNotExist()
        composeTestRule.onNodeWithTag("meerkat-import-insecure").assertDoesNotExist()
        composeTestRule.onNodeWithTag(
            "meerkat-import-start"
        ).performScrollTo().assertIsEnabled().performClick()
        assertEquals(1, starts)
    }

    @Test
    fun `an untested version blocks Start until try anyway is accepted`() {
        var accepted: Boolean? = null
        setContent(
            MeerkatImportUiState(connected = connected(supported = false)),
            onAcceptUntestedChange = { accepted = it }
        )
        composeTestRule.onNodeWithTag("meerkat-import-untested").assertIsDisplayed()
        composeTestRule.onNodeWithTag("meerkat-import-start").assertIsNotEnabled()
        composeTestRule.onNodeWithTag("meerkat-import-try-anyway").performScrollTo().performClick()
        assertEquals(true, accepted)
    }

    @Test
    fun `a known reason code shows the translated sentence, not the server's English reason`() {
        setContent(
            MeerkatImportUiState(
                connected = connected(
                    supported = false,
                    reason = "SERVER ENGLISH",
                    reasonCode = "newer_than_tested"
                )
            )
        )
        val node = composeTestRule.onNodeWithTag("meerkat-import-untested")
        node.assertTextContains("looks newer than the versions", substring = true)
        node.assert(!hasText("SERVER ENGLISH", substring = true))
    }

    @Test
    fun `the older reason code has its own translated sentence`() {
        setContent(
            MeerkatImportUiState(
                connected = connected(supported = false, reasonCode = "older_than_supported")
            )
        )
        composeTestRule.onNodeWithTag("meerkat-import-untested")
            .assertTextContains("looks older than v1.5", substring = true)
    }

    @Test
    fun `an unknown reason code falls back to the server's reason`() {
        setContent(
            MeerkatImportUiState(
                connected = connected(
                    supported = false,
                    reason = "SERVER ENGLISH",
                    reasonCode = "some_future_code"
                )
            )
        )
        composeTestRule.onNodeWithTag("meerkat-import-untested")
            .assertTextContains("SERVER ENGLISH", substring = true)
    }

    @Test
    fun `an untested version with try anyway accepted can start`() {
        setContent(
            MeerkatImportUiState(connected = connected(supported = false), acceptUntested = true)
        )
        composeTestRule.onNodeWithTag("meerkat-import-start").assertIsEnabled()
    }

    @Test
    fun `a plain-http connection shows the insecure warning`() {
        setContent(MeerkatImportUiState(connected = connected(insecure = true)))
        composeTestRule.onNodeWithTag("meerkat-import-insecure").assertIsDisplayed()
    }

    @Test
    fun `a Local profile shows the unavailable note instead of the form`() {
        setContent(MeerkatImportUiState(unavailable = true))
        composeTestRule.onNodeWithTag("meerkat-import-unavailable").assertIsDisplayed()
        composeTestRule.onNodeWithTag("meerkat-import-url").assertDoesNotExist()
    }

    @Test
    fun `the working step shows progress text`() {
        setContent(MeerkatImportUiState(step = MeerkatImportStep.WORKING, isImporting = true))
        composeTestRule.onNodeWithTag("meerkat-import-working").assertIsDisplayed()
    }

    private fun reviewState(loss: List<SourceImportIssue>) = MeerkatImportUiState(
        step = MeerkatImportStep.REVIEW,
        preview = SourceImportPreviewResponse(
            sessionId = "s1",
            rows = listOf(
                ImportRowPreview(
                    rowIndex = 0,
                    parsedContact = mapOf("firstname" to "Ada", "lastname" to "Lovelace"),
                    suggestedAction = "add"
                )
            ),
            totalRows = 1,
            validRows = 1,
            lossReport = loss
        ),
        rowActions = mapOf(0 to "add")
    )

    @Test
    fun `the review step lists rows and shows the loss summary only when there is loss`() {
        setContent(reviewState(emptyList()))
        composeTestRule.onNodeWithTag("import-review-list").assertIsDisplayed()
        composeTestRule.onNodeWithTag("meerkat-import-loss").assertDoesNotExist()
    }

    @Test
    fun `the review step shows the loss count`() {
        setContent(reviewState(listOf(SourceImportIssue("Ada", "photo", "dropped", "x"))))
        composeTestRule.onNodeWithTag("meerkat-import-loss").assertIsDisplayed()
    }

    @Test
    fun `the result step shows the outcome and Done fires`() {
        var done = 0
        setContent(
            MeerkatImportUiState(
                step = MeerkatImportStep.RESULT,
                result = SourceImportResult(created = 3, updated = 1, skipped = 2)
            ),
            onDone = { done++ }
        )
        composeTestRule.onNodeWithTag("meerkat-import-result").assertIsDisplayed()
        composeTestRule.onNodeWithTag("meerkat-import-done").performClick()
        assertEquals(1, done)
    }
}
