package com.mycorrhizal.crm.feature.contacts

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.Tag
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
        onCircleToggle: (String) -> Unit = {},
        onTagToggle: (String) -> Unit = {},
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactFormContent(
                    state = state,
                    onGivenNameChange = onGivenNameChange,
                    onSurnameChange = {},
                    onNicknameChange = {},
                    onEmailsChange = {},
                    onPhonesChange = {},
                    onAddressesChange = {},
                    onTitlesChange = {},
                    onImppChange = {},
                    onSocialChange = {},
                    onOtherServicesChange = {},
                    onLinksChange = {},
                    onPersonalInfoChange = {},
                    onBirthdayChange = {},
                    onNotesChange = {},
                    onCircleToggle = onCircleToggle,
                    onTagToggle = onTagToggle,
                    onSave = onSave,
                )
            }
        }
    }

    @Test
    fun `renders the form fields`() {
        setContent(
            state = ContactFormState(
                addresses = listOf(
                    com.mycorrhizal.crm.model.network.Address(
                        components = listOf(
                            com.mycorrhizal.crm.model.network.AddressComponent(kind = "name", value = "1 Main St"),
                        ),
                    ),
                ),
            ),
        )
        composeTestRule.onNodeWithText("Given name").assertIsDisplayed()
        composeTestRule.onNodeWithText("Surname").assertIsDisplayed()
        composeTestRule.onNodeWithText("Prefix").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Kind").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Language").performScrollTo().assertIsDisplayed()
        // M7: the previously read-only/invisible field groups now have editors.
        composeTestRule.onNodeWithText("Address").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Street").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Job titles").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Instant Messaging").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Social profiles").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Personal information").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("How we met").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Work information").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Additional contact information").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Birthday").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Notes").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("No circles yet").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("No tags yet").performScrollTo().assertIsDisplayed()
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

    @Test
    fun `a selected circle renders as a removable chip`() {
        var toggled: String? = null
        setContent(
            state = ContactFormState(
                circles = listOf("friends"),
                allCircles = listOf(Circle(id = "c1", name = "friends")),
            ),
            onCircleToggle = { toggled = it },
        )
        composeTestRule.onNodeWithText("friends").performScrollTo().performClick()
        assertEquals("friends", toggled)
    }

    @Test
    fun `a selected tag renders as a removable chip`() {
        var toggled: String? = null
        setContent(
            state = ContactFormState(
                tags = listOf("close"),
                allTags = listOf(Tag(id = "t1", name = "close")),
            ),
            onTagToggle = { toggled = it },
        )
        composeTestRule.onNodeWithText("close").performScrollTo().performClick()
        assertEquals("close", toggled)
    }
}
