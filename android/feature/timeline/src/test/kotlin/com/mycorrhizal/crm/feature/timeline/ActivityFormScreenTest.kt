package com.mycorrhizal.crm.feature.timeline

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import com.mycorrhizal.crm.model.network.ContactFlat
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

// M19: the activity form's multi-participant picker, driving the real
// ActivityFormContent — chips render and remove, the debounced search feeds
// results, and picking a result adds the participant.
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], qualifiers = "w480dp-h2000dp")
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ActivityFormScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(
        state: ActivityFormState,
        onSearchChange: (String) -> Unit = {},
        onAddParticipant: (ContactSummary) -> Unit = {},
        onRemoveParticipant: (Int) -> Unit = {},
        onSave: () -> Unit = {},
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ActivityFormContent(
                    state = state,
                    onTitleChange = {},
                    onTypeChange = {},
                    onDateChange = {},
                    onDescriptionChange = {},
                    onLocationChange = {},
                    onContactSearchChange = onSearchChange,
                    onAddParticipant = onAddParticipant,
                    onRemoveParticipant = onRemoveParticipant,
                    onSave = onSave,
                )
            }
        }
    }

    @Test
    fun `participant chips render and tapping one removes it`() {
        var removed: Int? = null
        setContent(
            ActivityFormState(
                contactId = 5,
                participants = listOf(ContactFlat(id = 5, firstname = "Dana"), ContactFlat(id = 9, firstname = "Bob")),
            ),
            onRemoveParticipant = { removed = it },
        )
        composeTestRule.onNodeWithText("Dana").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Bob").performScrollTo().assertIsDisplayed()

        composeTestRule.onNodeWithText("Dana").performScrollTo().performClick()
        assertEquals(5, removed)
    }

    @Test
    fun `typing in the search field invokes the contact search`() {
        var query = ""
        setContent(
            ActivityFormState(contactId = 5),
            onSearchChange = { query = it },
        )
        composeTestRule.onNodeWithText("Add participants").performScrollTo().performTextInput("dana")
        assertEquals("dana", query)
    }

    @Test
    fun `tapping a search result adds the participant`() {
        var added: ContactSummary? = null
        setContent(
            ActivityFormState(
                contactId = 5,
                contactSearchQuery = "dana",
                contactSearchResults = listOf(ContactSummary(id = 9, firstname = "Dana", lastname = "Lee")),
            ),
            onAddParticipant = { added = it },
        )
        composeTestRule.onNodeWithText("Dana Lee").performScrollTo().performClick()
        assertEquals(9, added?.id)
    }

    @Test
    fun `save button invokes the save callback`() {
        var saved = false
        setContent(
            ActivityFormState(contactId = 5, title = "Lunch"),
            onSave = { saved = true },
        )
        composeTestRule.onNodeWithText("Create activity").performScrollTo().performClick()
        assertEquals(true, saved)
    }
}
