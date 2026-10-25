package com.mycorrhizal.crm.feature.timelineentities

import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.hasContentDescription
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextReplacement
import com.mycorrhizal.crm.model.network.Gift
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issue #1773: every picker in the entity forms has a visible label and an
 * accessible name of "<label>, <value>" with the drop-down list role (what TalkBack
 * reads); the suggestion chips are labelled and tappable; gift and life event are
 * full-screen forms that save and leave the way the dialogs did.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class EntityFormsLabelsTest {
    @get:Rule
    val composeTestRule = createComposeRule()

    private val isDropdown = SemanticsMatcher.expectValue(
        SemanticsProperties.Role,
        Role.DropdownList
    )

    private fun giftForm(
        initial: Gift? = null,
        onConfirm: (GiftFormData) -> Unit = {},
        onDismiss: () -> Unit = {}
    ) = composeTestRule.setContent {
        MycorrhizalTheme {
            GiftForm(
                initial = initial,
                lifeEvents = emptyList(),
                activities = emptyList(),
                onConfirm = onConfirm,
                onDismiss = onDismiss
            )
        }
    }

    private fun preferenceForm() = composeTestRule.setContent {
        MycorrhizalTheme {
            PreferenceForm(initial = null, onConfirm = {}, onDismiss = {})
        }
    }

    @Test
    fun `gift status picker shows its label and is named label then value`() {
        giftForm()
        composeTestRule.onNodeWithText("Status").assertIsDisplayed()
        composeTestRule.onNodeWithTag("gift-status")
            .assert(hasContentDescription("Status, Idea"))
            .assert(isDropdown)
    }

    @Test
    fun `preference pickers are labelled and named with their current value`() {
        preferenceForm()
        composeTestRule.onNodeWithTag("preference-category")
            .assert(hasContentDescription("Category, Food")).assert(isDropdown)
        composeTestRule.onNodeWithTag("preference-sensitivity")
            .assert(hasContentDescription("Sensitivity, Normal")).assert(isDropdown)
    }

    @Test
    fun `life event pickers are labelled and the type is disabled until a category is chosen`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                LifeEventForm(
                    initial = null,
                    relatedContacts = emptyList(),
                    contactSearchQuery = "",
                    contactSearchResults = emptyList(),
                    contactSearchLoading = false,
                    onSearchContacts = {},
                    onAddRelated = {},
                    onRemoveRelated = {},
                    onConfirm = {},
                    onDismiss = {}
                )
            }
        }
        composeTestRule.onNodeWithTag("life-event-category")
            .assert(hasContentDescription("Category, Select a category…"))
        composeTestRule.onNodeWithTag("life-event-type")
            .assert(hasContentDescription("Event type, Select a category first"))
            .assertIsNotEnabled()
        // The pinned button says what enables it.
        composeTestRule.onNodeWithText("Choose an event type to continue").assertIsDisplayed()
    }

    @Test
    fun `an empty kind shows labelled suggestion chips that fill the field when tapped`() {
        preferenceForm()
        composeTestRule.onNodeWithText("Suggestions").assertIsDisplayed()
        composeTestRule.onNodeWithText("Kind").assertIsDisplayed()
        composeTestRule.onNodeWithText(
            "Allergy"
        ).performScrollTo().assertIsDisplayed().performClick()
        // Filled: the suggestions disappear and the field holds the choice.
        composeTestRule.onNodeWithText("Suggestions").assertDoesNotExist()
        composeTestRule.onNodeWithText("allergy").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `gift form create is blocked with a hint until a description is entered`() {
        var confirmed: GiftFormData? = null
        giftForm(onConfirm = { confirmed = it })
        composeTestRule.onNodeWithText("Enter a description to continue").assertIsDisplayed()
        composeTestRule.onNodeWithText("Create").assertIsNotEnabled()
        composeTestRule.onNodeWithText("Description").performTextReplacement("Socks")
        composeTestRule.onNodeWithText("Create").performClick()
        assertEquals("Socks", confirmed?.description)
    }

    @Test
    fun `the back arrow asks before discarding a typed gift`() {
        var confirmed = false
        var dismissed = false
        giftForm(onConfirm = { confirmed = true }, onDismiss = { dismissed = true })
        composeTestRule.onNodeWithText("New gift").assertIsDisplayed()
        composeTestRule.onNodeWithText("Description").performTextReplacement("Socks")
        composeTestRule.onNodeWithContentDescription("Back").performClick()

        // Issue #1751: the typed description is protected; Discard still leaves.
        composeTestRule.onNodeWithText("Discard changes?").assertIsDisplayed()
        assertEquals(false, dismissed)
        composeTestRule.onNodeWithText("Discard").performClick()
        assertEquals(true, dismissed)
        assertEquals(false, confirmed)
    }
}
