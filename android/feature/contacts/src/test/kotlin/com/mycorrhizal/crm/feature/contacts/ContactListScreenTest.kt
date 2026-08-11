package com.mycorrhizal.crm.feature.contacts

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
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

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ContactListScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(
        uiState: ContactListUiState,
        onContactClick: (Int) -> Unit = {},
        onSearchQueryChange: (String) -> Unit = {},
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactListScreenContent(
                    uiState = uiState,
                    onContactClick = onContactClick,
                    onSearchQueryChange = onSearchQueryChange,
                )
            }
        }
    }

    @Test
    fun `shows skeleton while loading`() {
        setContent(ContactListUiState(isLoading = true))
        composeTestRule.onNodeWithTag("contact-list-loading").assertIsDisplayed()
    }

    @Test
    fun `shows empty state when no contacts`() {
        setContent(ContactListUiState(isLoading = false, contacts = emptyList()))
        composeTestRule.onNodeWithText("No contacts yet").assertIsDisplayed()
    }

    @Test
    fun `shows contact list items`() {
        setContent(
            ContactListUiState(
                isLoading = false,
                contacts = listOf(
                    ContactSummary(id = 1, fn = "Alice", firstname = "Alice"),
                    ContactSummary(id = 2, fn = "Bob", firstname = "Bob"),
                ),
            ),
        )
        composeTestRule.onNodeWithText("Alice").assertIsDisplayed()
        composeTestRule.onNodeWithText("Bob").assertIsDisplayed()
    }

    @Test
    fun `tapping a contact navigates to its detail`() {
        var navigatedId: Int? = null
        setContent(
            ContactListUiState(
                isLoading = false,
                contacts = listOf(ContactSummary(id = 1, fn = "Alice", firstname = "Alice")),
            ),
            onContactClick = { navigatedId = it },
        )
        composeTestRule.onNodeWithText("Alice").performClick()
        assertEquals(1, navigatedId)
    }

    @Test
    fun `typing in the search field forwards the query`() {
        var query: String? = null
        setContent(
            ContactListUiState(isLoading = false, contacts = emptyList()),
            onSearchQueryChange = { query = it },
        )
        composeTestRule.onNodeWithText("Search contacts").performTextInput("ali")
        assertEquals("ali", query)
    }
}
