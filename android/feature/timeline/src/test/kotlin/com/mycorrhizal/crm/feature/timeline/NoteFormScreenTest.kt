package com.mycorrhizal.crm.feature.timeline

import androidx.compose.foundation.layout.padding
import androidx.compose.material3.SnackbarHostState
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import com.mycorrhizal.crm.ui.components.FormScaffold
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasScrollAction
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTouchInput
import androidx.compose.ui.test.swipeDown
import androidx.compose.ui.test.swipeUp
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

// M19: the note form's contact reassignment (web's EditTimelineItemDialog
// parity), driving the real NoteFormContent — the assigned-contact chip with
// its Clear action, and the debounced search-to-pick flow.
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], qualifiers = "w480dp-h2000dp")
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class NoteFormScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(
        state: NoteFormState,
        onSearchChange: (String) -> Unit = {},
        onPickContact: (ContactSummary) -> Unit = {},
        onClearContact: () -> Unit = {},
        onSave: () -> Unit = {},
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                FormScaffold(
                    title = "Form",
                    onBack = {},
                    saveLabel = if (state.isEdit) "Save changes" else "Create note",
                    isSaving = state.isSaving,
                    onSave = onSave,
                    snackbarHostState = remember { SnackbarHostState() },
                ) { padding ->
                    NoteFormContent(
                        state = state,
                        onContentChange = {},
                        onDateChange = {},
                        onContactSearchChange = onSearchChange,
                        onPickContact = onPickContact,
                        onClearContact = onClearContact,
                        modifier = Modifier.padding(padding),
                    )
                }
            }
        }
    }

    @Test
    fun `shows the assigned contact chip and its clear action`() {
        var cleared = false
        setContent(
            NoteFormState(contactId = 5, targetContactId = 9, targetContactName = "Dana Lee"),
            onClearContact = { cleared = true },
        )
        composeTestRule.onNodeWithText("Dana Lee").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Clear").performScrollTo().performClick()
        assertEquals(true, cleared)
    }

    @Test
    fun `typing in the search field invokes the contact search`() {
        var query = ""
        setContent(
            NoteFormState(contactId = 5),
            onSearchChange = { query = it },
        )
        composeTestRule.onNodeWithText("Search contacts").performScrollTo().performTextInput("dana")
        assertEquals("dana", query)
    }

    @Test
    fun `tapping a search result reassigns the note`() {
        var picked: ContactSummary? = null
        setContent(
            NoteFormState(
                contactId = 5,
                contactSearchQuery = "dana",
                contactSearchResults = listOf(ContactSummary(id = 9, firstname = "Dana", lastname = "Lee")),
            ),
            onPickContact = { picked = it },
        )
        composeTestRule.onNodeWithText("Dana Lee").performScrollTo().performClick()
        assertEquals(9, picked?.id)
    }

    @Test
    fun `save button invokes the save callback`() {
        var saved = false
        setContent(
            NoteFormState(contactId = 5, content = "Loves climbing"),
            onSave = { saved = true },
        )
        composeTestRule.onNodeWithText("Create note").performClick()
        assertEquals(true, saved)
    }

    // Issue #1404: the primary action is pinned in the Scaffold's bottomBar, so it
    // is on screen without scrolling and stays there as the form scrolls.
    @Test
    @Config(qualifiers = "w360dp-h400dp")
    fun `save button stays displayed without scrolling and while the form scrolls`() {
        var saved = false
        setContent(NoteFormState(contactId = 5, content = "Loves climbing"), onSave = { saved = true })

        composeTestRule.onNodeWithText("Create note").assertIsDisplayed()
        composeTestRule.onNode(hasScrollAction()).performTouchInput { swipeUp() }
        composeTestRule.onNodeWithText("Create note").assertIsDisplayed()
        composeTestRule.onNode(hasScrollAction()).performTouchInput { swipeDown(); swipeDown() }
        composeTestRule.onNodeWithText("Create note").assertIsDisplayed()
        composeTestRule.onNodeWithText("Create note").performClick()
        assertEquals(true, saved)
    }
}
