package com.mycorrhizal.crm.feature.contacts

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.SearchActivityHit
import com.mycorrhizal.crm.model.network.SearchNoteHit
import com.mycorrhizal.crm.model.network.SearchResult
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

    @Test
    fun `no cross-entity section renders when searchResult is null`() {
        setContent(
            ContactListUiState(isLoading = false, contacts = emptyList(), searchResult = null),
        )
        composeTestRule.onNodeWithText("matches in notes and activities", substring = true).assertDoesNotExist()
    }

    @Test
    fun `no cross-entity section renders for an empty result with no resolved relation`() {
        setContent(
            ContactListUiState(
                isLoading = false,
                contacts = emptyList(),
                searchResult = SearchResult(),
            ),
        )
        composeTestRule.onNodeWithText("matches in notes and activities", substring = true).assertDoesNotExist()
    }

    @Test
    fun `cross-entity header shows the total count and stays collapsed until tapped`() {
        setContent(
            ContactListUiState(
                isLoading = false,
                contacts = emptyList(),
                searchResult = SearchResult(
                    notes = listOf(SearchNoteHit(id = 1, content = "called mom", contactId = 5, contactName = "Dana")),
                    activities = listOf(SearchActivityHit(id = 2, title = "Coffee with mom")),
                ),
            ),
        )

        composeTestRule.onNodeWithText("2 matches in notes and activities").assertIsDisplayed()
        // Collapsed by default: the note/activity content itself isn't shown yet.
        composeTestRule.onNodeWithText("called mom").assertDoesNotExist()
        composeTestRule.onNodeWithText("Coffee with mom").assertDoesNotExist()

        composeTestRule.onNodeWithText("2 matches in notes and activities").performClick()

        composeTestRule.onNodeWithText("called mom").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Coffee with mom").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `a note with a contact shows its chip and navigates on tap`() {
        var navigatedId: Int? = null
        setContent(
            ContactListUiState(
                isLoading = false,
                contacts = emptyList(),
                searchResult = SearchResult(
                    notes = listOf(SearchNoteHit(id = 1, content = "called Dana", contactId = 5, contactName = "Dana White")),
                ),
            ),
            onContactClick = { navigatedId = it },
        )

        composeTestRule.onNodeWithText("1 matches in notes and activities").performClick()
        composeTestRule.onNodeWithText("Dana White").performScrollTo().performClick()

        assertEquals(5, navigatedId)
    }

    @Test
    fun `an unfiled note shows the unfiled chip instead of a contact`() {
        setContent(
            ContactListUiState(
                isLoading = false,
                contacts = emptyList(),
                searchResult = SearchResult(
                    notes = listOf(SearchNoteHit(id = 1, content = "a stray note", contactId = null, contactName = null)),
                ),
            ),
        )

        composeTestRule.onNodeWithText("1 matches in notes and activities").performClick()
        composeTestRule.onNodeWithText("Unfiled").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `a resolved relation banner shows even with zero hits`() {
        setContent(
            ContactListUiState(
                isLoading = false,
                contacts = emptyList(),
                searchResult = SearchResult(resolvedRelation = "parent_of"),
            ),
        )
        composeTestRule.onNodeWithText("Matched relationship: parent_of").assertIsDisplayed()
    }
}
