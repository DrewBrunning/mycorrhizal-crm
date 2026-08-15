package com.mycorrhizal.crm.feature.shares

import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.network.DuplicateMatch
import com.mycorrhizal.crm.model.network.ImportPreviewResponse
import com.mycorrhizal.crm.model.network.ImportRowPreview
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@OptIn(ExperimentalMaterial3Api::class)
class AcceptContactShareDialogTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(
        previewLoading: Boolean = false,
        preview: ImportPreviewResponse? = null,
        previewError: String? = null,
        confirmAction: String = "add",
        confirming: Boolean = false,
        onActionChange: (String) -> Unit = {},
        onConfirm: () -> Unit = {},
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                AcceptContactShareDialog(
                    previewLoading = previewLoading,
                    preview = preview,
                    previewError = previewError,
                    confirmAction = confirmAction,
                    confirming = confirming,
                    onActionChange = onActionChange,
                    onConfirm = onConfirm,
                    onDismiss = {},
                )
            }
        }
    }

    private fun preview(duplicate: DuplicateMatch? = null): ImportPreviewResponse = ImportPreviewResponse(
        sessionId = "import-1",
        rows = listOf(
            ImportRowPreview(
                rowIndex = 0,
                parsedContact = mapOf("firstname" to "Dana", "lastname" to "White"),
                duplicateMatch = duplicate,
                suggestedAction = if (duplicate != null) "update" else "add",
            ),
        ),
        totalRows = 1,
        validRows = 1,
        duplicateCount = if (duplicate != null) 1 else 0,
        errorCount = 0,
    )

    @Test
    fun `duplicate match renders its chip and the update action`() {
        setContent(
            preview = preview(
                duplicate = DuplicateMatch(
                    existingContactId = 3,
                    existingFirstname = "Dana",
                    existingLastname = "White",
                    matchReason = "name",
                ),
            ),
            confirmAction = "update",
        )

        composeTestRule.onNodeWithText("Matches: Dana White (name)").assertIsDisplayed()
        composeTestRule.onNodeWithText("Add as new contact").assertIsDisplayed()
        composeTestRule.onNodeWithText("Merge into existing contact").assertIsDisplayed()
        composeTestRule.onNodeWithText("Skip").assertIsDisplayed()
    }

    @Test
    fun `a share with no duplicate offers no update action`() {
        setContent(preview = preview(duplicate = null))

        composeTestRule.onNodeWithText("Add as new contact").assertIsDisplayed()
        composeTestRule.onNodeWithText("Merge into existing contact").assertDoesNotExist()
        composeTestRule.onNodeWithText("Skip").assertIsDisplayed()
    }

    @Test
    fun `confirm is disabled while loading`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                AcceptContactShareDialog(
                    previewLoading = true,
                    preview = null,
                    previewError = null,
                    confirmAction = "add",
                    confirming = false,
                    onActionChange = {},
                    onConfirm = {},
                    onDismiss = {},
                )
            }
        }
        composeTestRule.onNodeWithText("Confirm").assertIsNotEnabled()
    }

    @Test
    fun `confirm is disabled on preview error and the error is shown`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                AcceptContactShareDialog(
                    previewLoading = false,
                    preview = null,
                    previewError = "Payload could not be parsed",
                    confirmAction = "add",
                    confirming = false,
                    onActionChange = {},
                    onConfirm = {},
                    onDismiss = {},
                )
            }
        }
        composeTestRule.onNodeWithText("Payload could not be parsed").assertIsDisplayed()
        composeTestRule.onNodeWithText("Confirm").assertIsNotEnabled()
    }

    @Test
    fun `confirm is enabled once a preview row is loaded`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                AcceptContactShareDialog(
                    previewLoading = false,
                    preview = preview(),
                    previewError = null,
                    confirmAction = "add",
                    confirming = false,
                    onActionChange = {},
                    onConfirm = {},
                    onDismiss = {},
                )
            }
        }
        composeTestRule.onNodeWithText("Confirm").assertIsDisplayed()
        composeTestRule.onNodeWithText("Confirm").assertIsEnabled()
    }

    @Test
    fun `selecting an action reports it and confirm invokes the callback`() {
        var selected: String? = null
        var confirmed = false
        setContent(
            preview = preview(),
            onActionChange = { selected = it },
            onConfirm = { confirmed = true },
        )

        composeTestRule.onNodeWithText("Skip").performClick()
        composeTestRule.onNodeWithText("Confirm").performClick()

        assertEquals("skip", selected)
        assertTrue(confirmed)
    }
}
