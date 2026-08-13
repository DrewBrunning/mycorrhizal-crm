package com.mycorrhizal.crm.feature.contacts

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ContactFormScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(
        state: ContactFormState = ContactFormState(),
        onSave: () -> Unit = {},
        onGivenNameChange: (String) -> Unit = {},
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactFormContent(
                    state = state,
                    onGivenNameChange = onGivenNameChange,
                    onSurnameChange = {},
                    onNicknameChange = {},
                    onEmailValueChange = { _, _ -> },
                    onEmailAdd = {},
                    onEmailRemove = {},
                    onPhoneValueChange = { _, _ -> },
                    onPhoneAdd = {},
                    onPhoneRemove = {},
                    onBirthdayChange = {},
                    onNotesChange = {},
                    onCirclesTextChange = {},
                    onSave = onSave,
                )
            }
        }
    }

    @Test
    fun `renders the form fields`() {
        setContent()
        composeTestRule.onNodeWithText("Given name").assertIsDisplayed()
        composeTestRule.onNodeWithText("Surname").assertIsDisplayed()
        composeTestRule.onNodeWithText("Birthday").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Notes").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Circles (comma-separated)").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Create contact").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `typing a given name forwards the change`() {
        var name: String? = null
        setContent(onGivenNameChange = { name = it })
        composeTestRule.onNodeWithText("Given name").performTextInput("Carol")
        assertEquals("Carol", name)
    }

    @Test
    fun `edit mode shows the save label`() {
        setContent(state = ContactFormState(contactId = 5))
        composeTestRule.onNodeWithText("Save changes").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `save button invokes the callback`() {
        var saved = false
        setContent(onSave = { saved = true })
        composeTestRule.onNodeWithText("Create contact").performScrollTo().performClick()
        assertEquals(true, saved)
    }
}
